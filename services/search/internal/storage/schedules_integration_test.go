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
			job, found, err := store.ClaimSchedules(ctx, time.Minute)
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
	if _, found, err = store.ClaimSchedules(ctx, time.Minute); err != nil || found {
		t.Fatal("duplicate claim", err)
	}
	if _, err = owner.ExecContext(ctx, `UPDATE schedule_checks SET lease_until=now()-interval '1 second' WHERE request_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	newer, found, err := store.ClaimSchedules(ctx, time.Minute)
	if err != nil || !found || newer.Token == job.Token {
		t.Fatal("lease not recovered", err)
	}
	if err = store.FinishSchedules(ctx, job, &v1.ScheduleCheck{State: "done"}); err != nil {
		t.Fatal(err)
	}
	page, err := store.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PageSize: 1})
	if err != nil || page.ScheduleCheck.State != "running" {
		t.Fatal("late worker overwrote lease", err)
	}
	if err = store.FinishSchedules(ctx, newer, &v1.ScheduleCheck{State: "done", Schemes: []*v1.ScheduledScheme{{SchemeNumber: 1, State: "unverified"}}}); err != nil {
		t.Fatal(err)
	}
	page, err = store.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PageSize: 1})
	if err != nil || page.ScheduleCheck.State != "done" || len(page.ScheduleCheck.Schemes) != 1 {
		t.Fatal("missing saved check", err)
	}
	if _, found, err = store.ClaimSchedules(ctx, time.Minute); err != nil || found {
		t.Fatal("finished check repeated", err)
	}
	id = create()
	job, found, err = store.ClaimSchedules(ctx, time.Minute)
	if err != nil || !found {
		t.Fatal(err)
	}
	if err = store.Apply(ctx, consumer.Event{ID: uuid.NewString(), RequestID: id, Type: consumer.Cancelled, OccurredAt: time.Now(), Payload: []byte("cancel")}); err != nil {
		t.Fatal(err)
	}
	if active, err := store.SchedulesActive(ctx, job); err != nil || active {
		t.Fatal("cancel ignored", err)
	}
	if err = store.FinishSchedules(ctx, job, &v1.ScheduleCheck{State: "done"}); err != nil {
		t.Fatal(err)
	}
	page, err = store.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PageSize: 1})
	if err != nil || page.ScheduleCheck.State != "cancelled" || len(page.ScheduleCheck.Schemes) != 0 {
		t.Fatal("cancelled result overwritten", err)
	}
}
