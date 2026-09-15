package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func fixture(t *testing.T) kafka.Message {
	t.Helper()
	v := &eventsv1.TripRequestCreated{EventId: uuid.NewString(), RequestId: uuid.NewString(), SchemaVersion: 1, OccurredAt: timestamppb.Now(), Origin: &eventsv1.TripCity{Id: uuid.NewString(), Source: "geonames", SourceId: 1, Name: "Москва", CountryCode: "RU", Timezone: "Europe/Moscow"}, Destination: &eventsv1.TripCity{Id: uuid.NewString(), Source: "geonames", SourceId: 2, Name: "Казань", CountryCode: "RU", Timezone: "Europe/Moscow"}, DepartureFrom: "2027-01-01", DepartureTo: "2027-01-02", Adults: 1}
	b, err := proto.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return kafka.Message{Key: []byte(v.RequestId), Value: b}
}
func TestDecode(t *testing.T) {
	m := fixture(t)
	if _, err := Decode(m); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*kafka.Message)
	}{
		{"unknown", func(m *kafka.Message) { m.Headers = []kafka.Header{{Key: "event_type", Value: []byte("unknown")}} }},
		{"empty header", func(m *kafka.Message) { m.Headers = []kafka.Header{{Key: "event_type"}} }},
		{"wrong key", func(m *kafka.Message) { m.Key = []byte(uuid.NewString()) }},
		{"malformed", func(m *kafka.Message) { m.Value = []byte{255} }},
		{"duplicate header", func(m *kafka.Message) {
			m.Headers = []kafka.Header{{Key: "event_type", Value: []byte(Created)}, {Key: "event_type", Value: []byte(Created)}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := m
			tc.change(&v)
			if _, err := Decode(v); !errors.Is(err, ErrInvalidEvent) {
				t.Fatal(err)
			}
		})
	}
}

type fakeReader struct {
	message            kafka.Message
	fetched, committed int
	commitErr          error
	saved              *bool
}

func (r *fakeReader) FetchMessage(context.Context) (kafka.Message, error) {
	r.fetched++
	if r.fetched > 1 {
		return kafka.Message{}, errors.New("end")
	}
	return r.message, nil
}
func (r *fakeReader) CommitMessages(context.Context, ...kafka.Message) error {
	if !*r.saved {
		panic("commit before database")
	}
	r.committed++
	return r.commitErr
}

type fakeStore struct {
	saved bool
	err   error
}

func (s *fakeStore) Apply(context.Context, Event) error {
	if s.err == nil {
		s.saved = true
	}
	return s.err
}
func TestCommitOrder(t *testing.T) {
	for _, tc := range []string{"success", "db failure", "offset failure", "invalid event"} {
		t.Run(tc, func(t *testing.T) {
			s := &fakeStore{}
			r := &fakeReader{message: fixture(t), saved: &s.saved}
			if tc == "db failure" {
				s.err = errors.New("db down")
			}
			if tc == "offset failure" {
				r.commitErr = errors.New("lost ack")
			}
			if tc == "invalid event" {
				r.message.Value = nil
			}
			err := (Consumer{Reader: r, Repository: s, DBTimeout: time.Second, CommitTimeout: time.Second}).Run(context.Background())
			if err == nil {
				t.Fatal("expected stop")
			}
			want := 1
			if tc == "db failure" || tc == "invalid event" {
				want = 0
			}
			if r.committed != want {
				t.Fatalf("commits %d", r.committed)
			}
			if tc == "offset failure" && r.fetched != 1 {
				t.Fatal("advanced after failed commit")
			}
		})
	}
}
