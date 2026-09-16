//go:build integration

package auth

import (
	"context"
	"database/sql"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func testProgress(t *testing.T, ctx context.Context, app *sql.DB, user, origin, destination string) {
	t.Helper()
	s := storage.New(app)
	date := time.Now().AddDate(0, 0, 5).Format(time.DateOnly)
	id, err := s.CreateTrip(ctx, storage.Trip{UserID: user, RequestID: uuid.NewString(), OriginID: origin, DestinationID: destination, DepartureFrom: date, DepartureTo: date, Adults: 1})
	if err != nil {
		t.Fatal(err)
	}
	event := func(rev int64, stage eventsv1.RouteBuildingStage) *eventsv1.TripRouteBuildingUpdated {
		return &eventsv1.TripRouteBuildingUpdated{PlannerId: "graph", EventId: uuid.NewString(), SchemaVersion: 1, RequestId: id, Revision: rev, Stage: stage, OccurredAt: timestamppb.Now()}
	}
	apply := func(e *eventsv1.TripRouteBuildingUpdated) {
		t.Helper()
		b, err := proto.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ApplyProgress(ctx, e, b); err != nil {
			t.Fatal(err)
		}
	}
	ready := event(3, eventsv1.RouteBuildingStage_ROUTE_BUILDING_STAGE_AWAITING_SCHEDULES)
	ready.RouteCount = 4
	apply(ready)
	apply(ready)
	apply(event(2, eventsv1.RouteBuildingStage_ROUTE_BUILDING_STAGE_BUILDING))
	apply(event(1, eventsv1.RouteBuildingStage_ROUTE_BUILDING_STAGE_QUEUED))
	trip, err := s.GetTrip(ctx, user, id)
	if err != nil || trip.BuildingStage != "awaiting_schedules" || trip.Status != "running" || len(trip.History) != 3 {
		t.Fatal("projection regressed or history duplicated", err)
	}
	// A source's later revision must not advance or block the aggregate projection.
	source := event(10, eventsv1.RouteBuildingStage_ROUTE_BUILDING_STAGE_FAILED)
	source.SchemaVersion = 2
	source.PlannerId = "gemini"
	apply(source)
	aggregate := event(9, eventsv1.RouteBuildingStage_ROUTE_BUILDING_STAGE_BUILDING)
	aggregate.SchemaVersion = 2
	aggregate.PlannerId = "all"
	apply(aggregate)
	updated, readErr := s.GetTrip(ctx, user, id)
	if readErr != nil || updated.BuildingStage != "building" {
		t.Fatal("source event advanced aggregate revision", readErr)
	}
	conflict := proto.Clone(ready).(*eventsv1.TripRouteBuildingUpdated)
	conflict.RouteCount = 5
	b, _ := proto.Marshal(conflict)
	if s.ApplyProgress(ctx, conflict, b) == nil {
		t.Fatal("identity conflict accepted")
	}
	if err = s.CancelTrip(ctx, user, id); err != nil {
		t.Fatal(err)
	}
	apply(event(4, eventsv1.RouteBuildingStage_ROUTE_BUILDING_STAGE_BUILDING))
	trip, err = s.GetTrip(ctx, user, id)
	if err != nil || trip.Status != "cancelled" || len(trip.History) != 6 {
		t.Fatal("late result revived cancellation", err)
	}
	if _, err = s.GetTrip(ctx, uuid.NewString(), id); err == nil {
		t.Fatal("foreign history exposed")
	}
}
