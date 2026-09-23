//go:build integration

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/google/uuid"
)

func TestExpiryStopsWorkAndKeepsResults(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, db := testDB(t, ctx)
	s := New(db)
	event := func(kind, id string) consumer.Event {
		return consumer.Event{ID: uuid.NewString(), RequestID: id, Type: kind, OccurredAt: time.Now().UTC(), Payload: []byte("synthetic " + kind + id)}
	}
	apply := func(kind, id string) {
		t.Helper()
		if err := s.Apply(ctx, event(kind, id)); err != nil {
			t.Fatal(kind, err)
		}
	}
	state := func(id string) (status, stage string, expiredAt bool) {
		t.Helper()
		if err := db.QueryRowContext(ctx, `SELECT r.status,COALESCE(b.stage,''),r.expired_at IS NOT NULL FROM search_requests r LEFT JOIN route_building b USING(request_id) WHERE r.request_id=$1`, id).Scan(&status, &stage, &expiredAt); err != nil {
			t.Fatal(err)
		}
		return
	}
	// In-flight building: expiry drops the late result without a cancellation stage.
	running := uuid.NewString()
	apply(consumer.Created, running)
	job, ok, err := s.ClaimBuilding(ctx, time.Minute, 3)
	if err != nil || !ok || job.RequestID != running {
		t.Fatal("claim", err)
	}
	apply(consumer.Expired, running)
	if active, err := s.BuildingActive(ctx, job); err != nil || active {
		t.Fatal("expired building still active", err)
	}
	if err := s.FinishBuilding(ctx, job, "awaiting_schedules", realroutes.Result{Complete: true}); err != nil {
		t.Fatal(err)
	}
	if status, stage, at := state(running); status != "expired" || stage != "building" || !at {
		t.Fatal("late result or cancellation recorded", status, stage, at)
	}
	// Queued work is never claimed after expiry.
	queued := uuid.NewString()
	apply(consumer.Created, queued)
	apply(consumer.Expired, queued)
	if _, ok, err := s.ClaimBuilding(ctx, time.Minute, 3); err != nil || ok {
		t.Fatal("expired request claimed", err)
	}
	if status, stage, _ := state(queued); status != "expired" || stage != "queued" {
		t.Fatal("queued expiry", status, stage)
	}
	// Expiry before creation leaves a tombstone; creation never queues it.
	early := uuid.NewString()
	apply(consumer.Expired, early)
	apply(consumer.Created, early)
	if status, stage, _ := state(early); status != "expired" || stage != "" {
		t.Fatal("expiry tombstone", status, stage)
	}
	// Cancellation always wins, before or after expiry, and clears it.
	for _, expireFirst := range []bool{true, false} {
		id := uuid.NewString()
		apply(consumer.Created, id)
		if expireFirst {
			apply(consumer.Expired, id)
			apply(consumer.Cancelled, id)
		} else {
			apply(consumer.Cancelled, id)
			apply(consumer.Expired, id)
		}
		if status, _, at := state(id); status != "cancelled" || at {
			t.Fatal("cancellation lost to expiry", expireFirst, status, at)
		}
	}
}
