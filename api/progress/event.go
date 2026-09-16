// Package progress validates the versioned Search-to-Cabinet progress contract.
package progress

import (
	"errors"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"math"
)

const EventType = "travelwatch.events.v1.TripRouteBuildingUpdated"

var Stages = map[eventsv1.RouteBuildingStage]string{
	1: "queued", 2: "building", 3: "awaiting_schedules", 4: "no_routes", 5: "failed", 6: "cancelled",
}

func Decode(m kafka.Message) (*eventsv1.TripRouteBuildingUpdated, error) {
	count := 0
	for _, h := range m.Headers {
		if h.Key == "event_type" {
			if string(h.Value) != EventType {
				return nil, errors.New("unknown progress event")
			}
			count++
		}
	}
	e := new(eventsv1.TripRouteBuildingUpdated)
	if count != 1 || len(m.Value) > 16384 || proto.Unmarshal(m.Value, e) != nil || !validSource(e) || e.Revision < 1 || e.Revision == math.MaxInt64 || Stages[e.Stage] == "" || e.OccurredAt == nil || e.OccurredAt.CheckValid() != nil || e.Attempt < 0 || e.Attempt > 100 || e.DurationMs < 0 || e.RouteCount < 0 || e.RouteCount > 100 || (e.PlannerId != "all" && e.RouteCount > 50) {
		return nil, errors.New("invalid progress event")
	}
	if e.SchemaVersion == 2 && e.PlannerId != "all" && Stages[e.Stage] == "queued" {
		return nil, errors.New("source cannot emit queued aggregate")
	}
	switch e.Outcome {
	case "", "error", "timeout", "unavailable":
	default:
		return nil, errors.New("invalid progress outcome")
	}
	if e.Outcome != "" && (e.SchemaVersion != 2 || e.PlannerId == "all" || e.FinishedAt == nil || !e.Incomplete) {
		return nil, errors.New("unexpected progress outcome")
	}
	for _, v := range []string{e.EventId, e.RequestId} {
		id, err := uuid.Parse(v)
		if err != nil || id == uuid.Nil || id.String() != v {
			return nil, errors.New("invalid progress identity")
		}
	}
	if string(m.Key) != e.RequestId {
		return nil, errors.New("progress key mismatch")
	}
	if e.StartedAt != nil && e.StartedAt.CheckValid() != nil {
		return nil, errors.New("invalid start time")
	}
	if e.FinishedAt != nil && (e.FinishedAt.CheckValid() != nil || e.StartedAt == nil || e.FinishedAt.AsTime().Before(e.StartedAt.AsTime())) {
		return nil, errors.New("invalid finish time")
	}
	switch Stages[e.Stage] {
	case "queued":
		if e.Attempt != 0 || e.StartedAt != nil || e.FinishedAt != nil || e.RouteCount != 0 || e.DurationMs != 0 {
			return nil, errors.New("invalid queued state")
		}
	case "building":
		if e.Attempt < 1 || e.StartedAt == nil || e.FinishedAt != nil || e.RouteCount != 0 || e.DurationMs != 0 {
			return nil, errors.New("invalid building state")
		}
	case "awaiting_schedules", "no_routes", "failed":
		if e.Attempt < 1 || e.StartedAt == nil || e.FinishedAt == nil {
			return nil, errors.New("invalid final stage")
		}
	}
	if Stages[e.Stage] == "awaiting_schedules" && e.RouteCount == 0 {
		return nil, errors.New("ready without candidates")
	}
	if (Stages[e.Stage] == "no_routes" || Stages[e.Stage] == "failed") && e.RouteCount != 0 {
		return nil, errors.New("unexpected candidates")
	}
	return e, nil
}

// Version 1 is historical graph-only progress. Version 2 separates source
// observations from the aggregate barrier, identified by planner_id=all.
func validSource(e *eventsv1.TripRouteBuildingUpdated) bool {
	return (e.SchemaVersion == 1 && e.PlannerId == "graph") || (e.SchemaVersion == 2 && (e.PlannerId == "all" || e.PlannerId == "graph" || e.PlannerId == "gemini"))
}
