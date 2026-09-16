//go:build integration

package storage

import (
	"context"
	"github.com/NotaKronGit/travel-watch/api/progress"
	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"testing"
	"time"
)

func TestDurableRouteBuilding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, db := testDB(t, ctx)
	s := New(db)
	create := func() consumer.Event {
		return consumer.Event{ID: uuid.NewString(), RequestID: uuid.NewString(), Type: consumer.Created, OccurredAt: time.Now(), Payload: []byte("synthetic snapshot")}
	}
	e := create()
	if _, err := owner.ExecContext(ctx, `ALTER TABLE progress_outbox ADD CONSTRAINT fail_publish CHECK (false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if s.Apply(ctx, e) == nil {
		t.Fatal("event failure must roll back creation")
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM inbox_events WHERE event_id=$1`, e.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("inbox advanced without event", err)
	}
	if _, err := owner.ExecContext(ctx, `ALTER TABLE progress_outbox DROP CONSTRAINT fail_publish`); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(ctx, e); err != nil {
		t.Fatal(err)
	}
	first, ok, err := s.ClaimBuilding(ctx, time.Minute, 3)
	if err != nil || !ok {
		t.Fatal("claim", err)
	}
	if _, ok, err := s.ClaimBuilding(ctx, time.Minute, 3); err != nil || ok {
		t.Fatal("double claim", err)
	}
	if _, err := owner.ExecContext(ctx, `UPDATE route_building SET lease_until=now()-interval '1 second' WHERE request_id=$1`, e.RequestID); err != nil {
		t.Fatal(err)
	}
	second, ok, err := s.ClaimBuilding(ctx, time.Minute, 3)
	if err != nil || !ok || second.Token == first.Token || second.Attempt != 2 {
		t.Fatal("recovery", err)
	}
	result := realroutes.Result{Candidates: []realroutes.Candidate{{Steps: []realroutes.Step{{From: "synthetic", To: "synthetic destination"}}}}}
	if err := s.FinishBuilding(ctx, first, "awaiting_schedules", result); err != nil {
		t.Fatal(err)
	}
	var stage string
	if err := db.QueryRowContext(ctx, `SELECT stage FROM route_building WHERE request_id=$1`, e.RequestID).Scan(&stage); err != nil || stage != "building" {
		t.Fatal("stale attempt won", err)
	}
	if err := s.FinishBuilding(ctx, second, "awaiting_schedules", result); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishBuilding(ctx, second, "awaiting_schedules", result); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM progress_outbox WHERE request_id=$1`, e.RequestID).Scan(&n); err != nil || n != 4 {
		t.Fatal("duplicate or missing transitions", n, err)
	}
	m, ok, err := s.ClaimProgress(ctx, time.Minute)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err = progress.Decode(kafka.Message{Key: []byte(m.RequestID), Value: m.Payload, Headers: []kafka.Header{{Key: "event_type", Value: []byte(progress.EventType)}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.ExecContext(ctx, `UPDATE progress_outbox SET lease_until=now()-interval '1 second' WHERE event_id=$1`, m.ID); err != nil {
		t.Fatal(err)
	}
	replay, ok, err := s.ClaimProgress(ctx, time.Minute)
	if err != nil || !ok || replay.ID != m.ID || string(replay.Payload) != string(m.Payload) {
		t.Fatal("unstable delivery", err)
	}
	if err = s.AckProgress(ctx, m); err != nil {
		t.Fatal(err)
	}
	var published bool
	if err = db.QueryRowContext(ctx, `SELECT published_at IS NOT NULL FROM progress_outbox WHERE event_id=$1`, m.ID).Scan(&published); err != nil || published {
		t.Fatal("stale delivery ack", err)
	}
	if err = s.AckProgress(ctx, replay); err != nil {
		t.Fatal(err)
	}
	next := create()
	if err = s.Apply(ctx, next); err != nil {
		t.Fatal(err)
	}
	j, ok, err := s.ClaimBuilding(ctx, time.Minute, 3)
	if err != nil || !ok {
		t.Fatal(err)
	}
	stop := consumer.Event{ID: uuid.NewString(), RequestID: next.RequestID, Type: consumer.Cancelled, OccurredAt: time.Now(), Payload: []byte("cancel fixture")}
	if err = s.Apply(ctx, stop); err != nil {
		t.Fatal(err)
	}
	if active, err := s.BuildingActive(ctx, j); err != nil || active {
		t.Fatal("cancel not visible", err)
	}
	if err = s.FinishBuilding(ctx, j, "awaiting_schedules", result); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT stage FROM route_building WHERE request_id=$1`, next.RequestID).Scan(&stage); err != nil || stage != "cancelled" {
		t.Fatal("cancel revived", err)
	}
	// Exhausted crash retries become a recorded failure instead of running forever.
	last := create()
	if err = s.Apply(ctx, last); err != nil {
		t.Fatal(err)
	}
	j, ok, err = s.ClaimBuilding(ctx, time.Minute, 1)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err = owner.ExecContext(ctx, `UPDATE route_building SET lease_until=now()-interval '1 second' WHERE request_id=$1`, j.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = s.ClaimBuilding(ctx, time.Minute, 1); err != nil || ok {
		t.Fatal("unbounded attempts", err)
	}
	if err = db.QueryRowContext(ctx, `SELECT stage FROM route_building WHERE request_id=$1`, j.RequestID).Scan(&stage); err != nil || stage != "failed" {
		t.Fatal("lost failure", err)
	}
}
