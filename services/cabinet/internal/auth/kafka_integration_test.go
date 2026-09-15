//go:build integration && kafka

package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"os/exec"
	"testing"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/outbox"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

// failAck simulates a crash after Kafka accepted the message but before the DB ack.
var errLostAck = errors.New("test: lost DB acknowledgement")

type failAck struct{ *storage.Store }

func (f failAck) AckOutbox(context.Context, storage.OutboxMessage) error {
	return errLostAck
}

func TestKafkaOutbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	owner, app := integrationDB(t, ctx)
	store := storage.New(app)
	compose := func(ctx context.Context, args ...string) {
		t.Helper()
		command := exec.CommandContext(ctx, "docker", append([]string{"compose", "-p", "travel-watch-kafka-test", "-f", "../../../../deploy/test/compose.yaml"}, args...)...) //nolint:gosec // Fixed test project; arguments are constants supplied only by this test.
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("test broker command failed: %v: %s", err, output)
		}
	}
	topic := "test-outbox-" + hex.EncodeToString(randBytes(8))
	conn, err := kafka.DialContext(ctx, "tcp", "127.0.0.1:19092")
	if err != nil {
		t.Fatal("run make test-kafka", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
		t.Fatal(err)
	}
	// Unique test topics are deleted after assertions; the test broker has no user topics.
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		client := &kafka.Client{Addr: kafka.TCP("127.0.0.1:19092")}
		response, err := client.DeleteTopics(cleanup, &kafka.DeleteTopicsRequest{Topics: []string{topic}})
		if err != nil {
			t.Error(err)
		} else if response.Errors[topic] != nil {
			t.Error(response.Errors[topic])
		}
	})
	// Only the dedicated test project is stopped; restore it even after a failed assertion.
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 45*time.Second)
		defer c()
		compose(cleanup, "up", "-d", "--wait", "--wait-timeout", "40", "kafka")
	})
	var user string
	if err := owner.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES('kafka-test@example.com','test-only') RETURNING id").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ExecContext(ctx, "INSERT INTO catalog_countries(code,name,name_ru) VALUES('RU','Test country','Тестовая страна')"); err != nil {
		t.Fatal(err)
	}
	origin := "11111111-1111-1111-1111-111111111111"
	destination := "22222222-2222-2222-2222-222222222222"
	for i, id := range []string{origin, destination} {
		if _, err := owner.ExecContext(ctx, "INSERT INTO catalog_cities(id,source,source_id,name,name_ru,country_code,region_code,timezone,aliases,latitude,longitude,population,active) VALUES($1,'kafka-test',$2,'Fixture','Тестовый город','RU','01','Europe/Moscow','[]',0,0,0,true)", id, i+1); err != nil {
			t.Fatal(err)
		}
	}
	date := time.Now().AddDate(0, 0, 5).Format(time.DateOnly)
	cfg := config.Outbox{Brokers: []string{"127.0.0.1:19092"}, Topic: topic, DBTimeout: time.Second, PublishTimeout: 3 * time.Second, LeaseDuration: 10 * time.Second, PollInterval: time.Millisecond * 100, RetryMin: time.Millisecond * 100, RetryMax: time.Second}
	publisher := outbox.NewKafkaPublisher(cfg)
	defer publisher.Close()
	relay := outbox.Relay{Repository: store, Publisher: publisher, Config: cfg}
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: cfg.Brokers, Topic: topic, Partition: 0, MinBytes: 1, MaxBytes: 1 << 20, MaxWait: time.Second})
	defer reader.Close()
	read := func() kafka.Message {
		t.Helper()
		readctx, c := context.WithTimeout(ctx, 15*time.Second)
		defer c()
		m, err := reader.ReadMessage(readctx)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	step := func() {
		t.Helper()
		for {
			found, err := relay.Step(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if found {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	compose(ctx, "stop", "-t", "5", "kafka")
	id, err := store.CreateTrip(ctx, storage.Trip{UserID: user, RequestID: "33333333-3333-3333-3333-333333333333", OriginID: origin, DestinationID: destination, DepartureFrom: date, DepartureTo: date, Adults: 1})
	if err != nil {
		t.Fatal("trip depends on Kafka", err)
	}
	if found, err := relay.Step(ctx); !found || err == nil {
		t.Fatal("offline Kafka accepted", err)
	}
	var published sql.NullTime
	var attempts int
	var payload []byte
	if err := app.QueryRowContext(ctx, "SELECT published_at,attempts,payload FROM outbox_events WHERE request_id=$1", id).Scan(&published, &attempts, &payload); err != nil || published.Valid || attempts != 1 {
		t.Fatal("offline event lost", err)
	}
	compose(ctx, "up", "-d", "--wait", "--wait-timeout", "40", "kafka")
	relay.Repository = failAck{store}
	// The broker may briefly refresh metadata after a restart: keep retrying until a real ack is lost.
	for {
		_, err := relay.Step(ctx)
		if errors.Is(err, errLostAck) {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(100 * time.Millisecond)
	}
	first := read()
	if !bytes.Equal(first.Value, payload) || string(first.Key) != id {
		t.Fatal("Kafka payload/key differ from outbox")
	}
	var event eventsv1.TripRequestCreated
	if err := proto.Unmarshal(first.Value, &event); err != nil || event.RequestId != id || event.EventId == "" {
		t.Fatal("invalid protobuf", err)
	}
	// Expire the lost-ack lease without waiting ten seconds; retry must use the identical bytes.
	if _, err := owner.ExecContext(ctx, "UPDATE outbox_events SET lease_until=now()-interval '1 second' WHERE request_id=$1", id); err != nil {
		t.Fatal(err)
	}
	relay.Repository = store
	step()
	second := read()
	if !bytes.Equal(first.Value, second.Value) || !bytes.Equal(first.Key, second.Key) {
		t.Fatal("redelivery changed event")
	}
	if err := app.QueryRowContext(ctx, "SELECT published_at FROM outbox_events WHERE request_id=$1", id).Scan(&published); err != nil || !published.Valid {
		t.Fatal("successful delivery not acknowledged", err)
	}
	if found, err := relay.Step(ctx); err != nil || found {
		t.Fatal("published event claimed again", err)
	}
	// Kafka data survives a container restart on the named volume.
	compose(ctx, "restart", "kafka")
	compose(ctx, "up", "-d", "--wait", "--wait-timeout", "40", "kafka")
	persisted := kafka.NewReader(kafka.ReaderConfig{Brokers: cfg.Brokers, Topic: topic, Partition: 0, MinBytes: 1, MaxBytes: 1 << 20})
	defer persisted.Close()
	m, err := persisted.ReadMessage(ctx)
	if err != nil || !bytes.Equal(m.Value, payload) {
		t.Fatal("Kafka data lost on restart", err)
	}
}
