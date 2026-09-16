//go:build integration

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/NotaKronGit/travel-watch/services/search/internal/planning"
	"github.com/google/uuid"
)

func TestSourcesBarrierRecoveryAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, db := testDB(t, ctx)
	s := NewBuilder(db, []string{"graph", "gemini"})
	create := func() string {
		t.Helper()
		id := uuid.NewString()
		if err := s.Apply(ctx, consumer.Event{ID: uuid.NewString(), RequestID: id, Type: consumer.Created, OccurredAt: time.Now(), Payload: []byte("snapshot")}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stage := func(id string) string {
		t.Helper()
		var stage string
		if err := db.QueryRowContext(ctx, `SELECT stage FROM route_building WHERE request_id=$1`, id).Scan(&stage); err != nil {
			t.Fatal(err)
		}
		return stage
	}
	claim := func(store *Store) planning.Job {
		t.Helper()
		j, ok, err := store.ClaimBuilding(ctx, time.Minute, 2)
		if err != nil || !ok {
			t.Fatal(ok, err)
		}
		return j
	}
	id := create()
	first := claim(s)
	if len(first.Sources) != 2 {
		t.Fatal(first)
	}
	if _, err := owner.ExecContext(ctx, `ALTER TABLE progress_outbox ADD CONSTRAINT fail_source CHECK(false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if s.FinishSource(ctx, first, "gemini", planning.SourceResult{Count: 2}) == nil {
		t.Fatal("source committed without event")
	}
	var sourceStage string
	if err := db.QueryRowContext(ctx, `SELECT stage FROM planner_runs WHERE request_id=$1 AND planner_id='gemini'`, id).Scan(&sourceStage); err != nil || sourceStage != "building" {
		t.Fatal("source transaction not rolled back", err)
	}
	if _, err := owner.ExecContext(ctx, `ALTER TABLE progress_outbox DROP CONSTRAINT fail_source`); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSource(ctx, first, "gemini", planning.SourceResult{Count: 2, Incomplete: true}); err != nil {
		t.Fatal(err)
	}
	if stage(id) != "building" {
		t.Fatal("advanced before all sources finished")
	}
	var rev int64
	_ = db.QueryRowContext(ctx, `SELECT revision FROM route_building WHERE request_id=$1`, id).Scan(&rev)
	if err := s.FinishSource(ctx, first, "gemini", planning.SourceResult{Count: 5}); err != nil {
		t.Fatal(err)
	}
	var after int64
	_ = db.QueryRowContext(ctx, `SELECT revision FROM route_building WHERE request_id=$1`, id).Scan(&after)
	if rev != after {
		t.Fatal("duplicate source created history")
	}
	if _, err := owner.ExecContext(ctx, `UPDATE route_building SET lease_until=now()-interval '1 second' WHERE request_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	// Changed config cannot remove graph or rerun completed Gemini.
	second := claim(NewBuilder(db, []string{"gemini"}))
	if len(second.Sources) != 1 || second.Sources[0] != "graph" {
		t.Fatal(second)
	}
	if err := s.FinishSource(ctx, first, "graph", planning.SourceResult{Count: 9}); err != nil {
		t.Fatal(err)
	}
	if stage(id) != "building" {
		t.Fatal("stale token advanced state")
	}
	if err := s.FinishSource(ctx, second, "graph", planning.SourceResult{Outcome: "timeout", Incomplete: true}); err != nil {
		t.Fatal(err)
	}
	if stage(id) != "awaiting_schedules" {
		t.Fatal("lost saved Gemini result")
	}
	for _, tc := range []struct{ name, outcome, want string }{{"empty", "", "no_routes"}, {"failed", "error", "failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			id := create()
			j := claim(s)
			for _, source := range j.Sources {
				if err := s.FinishSource(ctx, j, source, planning.SourceResult{Outcome: tc.outcome}); err != nil {
					t.Fatal(err)
				}
			}
			if stage(id) != tc.want {
				t.Fatal(stage(id))
			}
		})
	}
	id = create()
	j := claim(s)
	if err := s.Apply(ctx, consumer.Event{ID: uuid.NewString(), RequestID: id, Type: consumer.Cancelled, OccurredAt: time.Now(), Payload: []byte("cancel")}); err != nil {
		t.Fatal(err)
	}
	for _, source := range j.Sources {
		if err := s.FinishSource(ctx, j, source, planning.SourceResult{Count: 3}); err != nil {
			t.Fatal(err)
		}
	}
	if stage(id) != "cancelled" {
		t.Fatal("cancelled job resumed")
	}
	id = create()
	j = claim(s)
	if err := s.FinishSource(ctx, j, "gemini", planning.SourceResult{Count: 1, Incomplete: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ExecContext(ctx, `UPDATE route_building SET attempt=2,lease_until=now()-interval '1 second' WHERE request_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.ClaimBuilding(ctx, time.Minute, 2); err != nil || ok {
		t.Fatal("exhaustion", ok, err)
	}
	if stage(id) != "awaiting_schedules" {
		t.Fatal("exhaustion lost completed source")
	}
	id = create()
	j = claim(s)
	for _, source := range j.Sources {
		if err := s.FinishSource(ctx, j, source, planning.SourceResult{Count: 2}); err != nil {
			t.Fatal(err)
		}
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT sum(route_count) FROM planner_runs WHERE request_id=$1`, id).Scan(&total); err != nil || total != 4 || stage(id) != "awaiting_schedules" {
		t.Fatal("two successful sources", total, err)
	}
}
