package progress

import (
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func TestStrictProgressContract(t *testing.T) {
	now := time.Now()
	base := &eventsv1.TripRouteBuildingUpdated{PlannerId: "graph", EventId: uuid.NewString(), SchemaVersion: 1, RequestId: uuid.NewString(), Revision: 3, Stage: 3, Attempt: 1, OccurredAt: timestamppb.New(now), StartedAt: timestamppb.New(now.Add(-time.Second)), FinishedAt: timestamppb.New(now), DurationMs: 1000, RouteCount: 4, Incomplete: true}
	decode := func(e *eventsv1.TripRouteBuildingUpdated) error {
		b, _ := proto.Marshal(e)
		_, err := Decode(kafka.Message{Key: []byte(e.RequestId), Value: b, Headers: []kafka.Header{{Key: "event_type", Value: []byte(EventType)}}})
		return err
	}
	if err := decode(base); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*eventsv1.TripRouteBuildingUpdated){func(e *eventsv1.TripRouteBuildingUpdated) { e.RouteCount = 0 }, func(e *eventsv1.TripRouteBuildingUpdated) { e.SchemaVersion = 99 }, func(e *eventsv1.TripRouteBuildingUpdated) { e.Stage = 99 }, func(e *eventsv1.TripRouteBuildingUpdated) { e.Revision = 0 }, func(e *eventsv1.TripRouteBuildingUpdated) { e.FinishedAt = timestamppb.New(now.Add(-time.Hour)) }} {
		e := proto.Clone(base).(*eventsv1.TripRouteBuildingUpdated)
		change(e)
		if decode(e) == nil {
			t.Fatal("invalid progress accepted")
		}
	}
	b, _ := proto.Marshal(base)
	if _, err := Decode(kafka.Message{Key: []byte(base.RequestId), Value: b}); err == nil {
		t.Fatal("missing type accepted")
	}
}

func TestMultiSourceProgress(t *testing.T) {
	now := timestamppb.Now()
	for _, id := range []string{"all", "graph", "gemini"} {
		e := &eventsv1.TripRouteBuildingUpdated{SchemaVersion: 2, PlannerId: id, EventId: uuid.NewString(), RequestId: uuid.NewString(), Revision: 1, Stage: 5, Attempt: 1, OccurredAt: now, StartedAt: now, FinishedAt: now, Incomplete: true}
		if id != "all" {
			e.Outcome = "timeout"
		}
		b, _ := proto.Marshal(e)
		if _, err := Decode(kafka.Message{Key: []byte(e.RequestId), Value: b, Headers: []kafka.Header{{Key: "event_type", Value: []byte(EventType)}}}); err != nil {
			t.Fatal(id, err)
		}
	}
}
