//go:build integration

package storage

import (
	"context"
	"encoding/json"
	"github.com/NotaKronGit/travel-watch/services/search/internal/schedules"
	"sync"
	"testing"
	"time"

	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/NotaKronGit/travel-watch/services/search/internal/planning"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/google/uuid"
)

func TestScheduleClaimsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, db := testDB(t, ctx)
	store := NewBuilder(db, []string{"graph"})
	create := func() string {
		t.Helper()
		id := uuid.NewString()
		if err := store.Apply(ctx, consumer.Event{ID: uuid.NewString(), RequestID: id, Type: consumer.Created, OccurredAt: time.Now(), Payload: []byte("fixture")}); err != nil {
			t.Fatal(err)
		}
		job, found, err := store.ClaimBuilding(ctx, time.Minute, 3)
		if err != nil || !found {
			t.Fatal(err)
		}
		data, _ := json.Marshal(realroutes.Result{Candidates: []realroutes.Candidate{{Steps: []realroutes.Step{{From: "A", To: "B", Mode: "plane"}}}}})
		if err = store.FinishSource(ctx, job, "graph", planning.SourceResult{Data: data, Count: 1}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	id := create()
	var wg sync.WaitGroup
	winners := make(chan schedules.Job, 8)
	for range 8 {
		wg.Go(func() {
			job, found, err := store.ClaimSchedules(ctx, time.Minute, 0)
			if err != nil {
				t.Error(err)
			}
			if found {
				winners <- job
			}
		})
	}
	wg.Wait()
	close(winners)
	var job schedules.Job
	count := 0
	for winner := range winners {
		job = winner
		count++
	}
	if count != 1 || job.RequestID != id {
		t.Fatal("concurrent claims", count)
	}
	var found bool
	var err error
	if _, found, err = store.ClaimSchedules(ctx, time.Minute, 0); err != nil || found {
		t.Fatal("duplicate claim", err)
	}
	if _, err = owner.ExecContext(ctx, `UPDATE schedule_checks SET lease_until=now()-interval '1 second' WHERE request_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	newer, found, err := store.ClaimSchedules(ctx, time.Minute, 0)
	if err != nil || !found || newer.Token == job.Token {
		t.Fatal("lease not recovered", err)
	}
	if err = store.FinishSchedules(ctx, job, &v1.ScheduleCheck{State: "done"}, 0); err != nil {
		t.Fatal(err)
	}
	page, err := store.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PageSize: 1})
	if err != nil || page.ScheduleCheck.State != "running" {
		t.Fatal("late worker overwrote lease", err)
	}
	if err = store.FinishSchedules(ctx, newer, &v1.ScheduleCheck{State: "done", Schemes: []*v1.ScheduledScheme{{SchemeNumber: 1, State: "unverified"}}}, 0); err != nil {
		t.Fatal(err)
	}
	page, err = store.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PageSize: 1})
	if err != nil || page.ScheduleCheck.State != "done" || len(page.ScheduleCheck.Schemes) != 1 {
		t.Fatal("missing saved check", err)
	}
	if _, found, err = store.ClaimSchedules(ctx, time.Minute, 0); err != nil || found {
		t.Fatal("finished check repeated", err)
	}
	id = create()
	job, found, err = store.ClaimSchedules(ctx, time.Minute, 0)
	if err != nil || !found {
		t.Fatal(err)
	}
	if err = store.Apply(ctx, consumer.Event{ID: uuid.NewString(), RequestID: id, Type: consumer.Cancelled, OccurredAt: time.Now(), Payload: []byte("cancel")}); err != nil {
		t.Fatal(err)
	}
	if active, err := store.SchedulesActive(ctx, job); err != nil || active {
		t.Fatal("cancel ignored", err)
	}
	if err = store.FinishSchedules(ctx, job, &v1.ScheduleCheck{State: "done"}, 0); err != nil {
		t.Fatal(err)
	}
	page, err = store.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PageSize: 1})
	if err != nil || page.ScheduleCheck.State != "cancelled" || len(page.ScheduleCheck.Schemes) != 0 {
		t.Fatal("cancelled result overwritten", err)
	}
}

func TestScheduleRecheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, db := testDB(t, ctx)
	store := NewBuilder(db, []string{"graph"})
	create := func() string {
		t.Helper()
		id := uuid.NewString()
		if err := store.Apply(ctx, consumer.Event{ID: uuid.NewString(), RequestID: id, Type: consumer.Created, OccurredAt: time.Now(), Payload: []byte("fixture")}); err != nil {
			t.Fatal(err)
		}
		job, found, err := store.ClaimBuilding(ctx, time.Minute, 3)
		if err != nil || !found {
			t.Fatal(err)
		}
		data, _ := json.Marshal(realroutes.Result{Candidates: []realroutes.Candidate{{Steps: []realroutes.Step{{From: "A", To: "B", Mode: "plane"}}}}})
		if err = store.FinishSource(ctx, job, "graph", planning.SourceResult{Data: data, Count: 1}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	read := func(id string) *v1.ScheduleCheck {
		t.Helper()
		r, err := store.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		return r.ScheduleCheck
	}
	due := func(id string) {
		t.Helper()
		if _, err := owner.ExecContext(ctx, "UPDATE schedule_checks SET next_check_at=now()-interval '1 second' WHERE request_id=$1", id); err != nil {
			t.Fatal(err)
		}
	}
	claim := func() (schedules.Job, bool) {
		t.Helper()
		job, found, err := store.ClaimSchedules(ctx, time.Minute, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return job, found
	}
	schemes := func(n int32) *v1.ScheduleCheck {
		return &v1.ScheduleCheck{State: "done", Schemes: []*v1.ScheduledScheme{{SchemeNumber: n, State: "compatible"}}}
	}
	id := create()
	job, found := claim()
	if !found || job.RequestID != id {
		t.Fatal("first check not claimed")
	}
	if err := store.FinishSchedules(ctx, job, schemes(1), time.Hour); err != nil {
		t.Fatal(err)
	}
	got := read(id)
	if got.NextCheckAt == nil || time.Until(got.NextCheckAt.AsTime()) < 59*time.Minute {
		t.Fatal("next check not planned an hour ahead", got.NextCheckAt)
	}
	if _, found = claim(); found {
		t.Fatal("re-checked before due")
	}
	// A due re-check keeps the previous result readable while it runs.
	due(id)
	job, found = claim()
	if !found || job.RequestID != id {
		t.Fatal("due re-check not claimed")
	}
	if got = read(id); got.State != "running" || len(got.Schemes) != 1 {
		t.Fatal("previous result hidden during re-check", got)
	}
	// A failed re-check keeps the previous result and records the failure.
	if err := store.FinishSchedules(ctx, job, &v1.ScheduleCheck{State: "failed", Incomplete: true}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if got = read(id); got.State != "done" || len(got.Schemes) != 1 || got.RefreshFailedAt == nil || got.NextCheckAt == nil {
		t.Fatal("failed re-check erased the result", got)
	}
	// A successful one replaces it and clears the failure.
	due(id)
	job, _ = claim()
	if err := store.FinishSchedules(ctx, job, schemes(2), time.Hour); err != nil {
		t.Fatal(err)
	}
	if got = read(id); got.Schemes[0].SchemeNumber != 2 || got.RefreshFailedAt != nil {
		t.Fatal("re-check result not stored", got)
	}
	// Never-checked requests go before due re-checks.
	due(id)
	fresh := create()
	if job, found = claim(); !found || job.RequestID != fresh {
		t.Fatal("new request waited behind a re-check", job.RequestID)
	}
	if err := store.FinishSchedules(ctx, job, schemes(1), 0); err != nil {
		t.Fatal(err)
	}
	// With re-checks disabled nothing is planned for that request.
	if got = read(fresh); got.NextCheckAt != nil {
		t.Fatal("re-check planned while disabled")
	}
	// Expired requests are not re-checked and show no next check.
	if err := store.Apply(ctx, consumer.Event{ID: uuid.NewString(), RequestID: id, Type: consumer.Expired, OccurredAt: time.Now(), Payload: []byte("expired")}); err != nil {
		t.Fatal(err)
	}
	if _, found = claim(); found {
		t.Fatal("expired request re-checked")
	}
	if got = read(id); got.NextCheckAt != nil || len(got.Schemes) != 1 {
		t.Fatal("expired request result", got)
	}
}
