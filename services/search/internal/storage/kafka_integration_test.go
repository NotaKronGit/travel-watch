//go:build integration && kafka

package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type interruptedReader struct {
	*kafka.Reader
	fail    bool
	commits int
	stop    context.CancelFunc
}

func (r *interruptedReader) CommitMessages(ctx context.Context, m ...kafka.Message) error {
	if r.fail {
		return errors.New("simulated interruption before offset commit")
	}
	if err := r.Reader.CommitMessages(ctx, m...); err != nil {
		return err
	}
	r.commits++
	if r.commits == 3 {
		r.stop()
	}
	return nil
}
func TestKafkaInbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, db := testDB(t, ctx)
	topic := "test-search-" + uuid.NewString()
	conn, err := kafka.DialContext(ctx, "tcp", "127.0.0.1:19092")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, c := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer c()
		client := &kafka.Client{Addr: kafka.TCP("127.0.0.1:19092")}
		res, err := client.DeleteTopics(clean, &kafka.DeleteTopicsRequest{Topics: []string{topic}})
		if err != nil {
			t.Error(err)
		} else if res.Errors[topic] != nil {
			t.Error(res.Errors[topic])
		}
	})
	id := uuid.NewString()
	stop := &eventsv1.TripRequestCancelled{EventId: uuid.NewString(), RequestId: id, SchemaVersion: 1, OccurredAt: timestamppb.Now()}
	create := &eventsv1.TripRequestCreated{EventId: uuid.NewString(), RequestId: id, SchemaVersion: 1, OccurredAt: timestamppb.Now(), Origin: &eventsv1.TripCity{Id: uuid.NewString(), Source: "fixture", SourceId: 1, Name: "Москва", CountryCode: "RU", Timezone: "Europe/Moscow"}, Destination: &eventsv1.TripCity{Id: uuid.NewString(), Source: "fixture", SourceId: 2, Name: "Казань", CountryCode: "RU", Timezone: "Europe/Moscow"}, DepartureFrom: "2027-01-01", DepartureTo: "2027-01-02", Adults: 1}
	marshal := func(v proto.Message) []byte {
		t.Helper()
		b, err := proto.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cancellation := kafka.Message{Key: []byte(id), Value: marshal(stop), Headers: []kafka.Header{{Key: "event_type", Value: []byte(consumer.Cancelled)}}}
	creation := kafka.Message{Key: []byte(id), Value: marshal(create)} // Legacy headerless creation.
	w := &kafka.Writer{Addr: kafka.TCP("127.0.0.1:19092"), Topic: topic, RequiredAcks: kafka.RequireAll, WriteTimeout: 10 * time.Second, ReadTimeout: 10 * time.Second}
	defer w.Close()
	if err = w.WriteMessages(ctx, cancellation, creation, creation); err != nil {
		t.Fatal(err)
	}
	cfg := config.Consumer{Brokers: []string{"127.0.0.1:19092"}, Topic: topic, GroupID: "test-" + uuid.NewString(), DialTimeout: time.Second}
	first := &interruptedReader{Reader: consumer.NewReader(cfg), fail: true}
	err = (consumer.Consumer{Reader: first, Repository: New(db), DBTimeout: time.Second, CommitTimeout: 5 * time.Second}).Run(ctx)
	if err == nil {
		t.Fatal("expected interrupted commit")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM inbox_events").Scan(&n); err != nil || n != 1 {
		t.Fatal("database commit missing before restart", n, err)
	}
	runctx, stopRun := context.WithCancel(ctx)
	defer stopRun()
	second := &interruptedReader{Reader: consumer.NewReader(cfg), stop: stopRun}
	defer second.Close()
	_ = (consumer.Consumer{Reader: second, Repository: New(db), DBTimeout: time.Second, CommitTimeout: 5 * time.Second}).Run(runctx)
	if second.commits != 3 {
		t.Fatalf("expected replay and subsequent messages, got %d", second.commits)
	}
	var status string
	var payload []byte
	if err = db.QueryRowContext(ctx, "SELECT status,created_payload FROM search_requests WHERE request_id=$1", id).Scan(&status, &payload); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" || len(payload) == 0 {
		t.Fatal("late creation reactivated or lost snapshot")
	}
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM inbox_events").Scan(&n); err != nil || n != 2 {
		t.Fatal("duplicate inbox", n, err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	// A fresh group member resumes after the last committed offset.
	third := consumer.NewReader(cfg)
	defer third.Close()
	check, stopCheck := context.WithTimeout(ctx, 5*time.Second)
	defer stopCheck()
	if _, err = third.FetchMessage(check); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("committed events replayed", err)
	}
}
