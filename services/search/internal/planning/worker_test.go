package planning

import (
	"context"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"testing"
	"time"
)

type fakeRepository struct {
	active   bool
	stage    string
	finished bool
}

func (f *fakeRepository) ClaimBuilding(context.Context, time.Duration, int) (Job, bool, error) {
	return Job{}, true, nil
}
func (f *fakeRepository) BuildingActive(context.Context, Job) (bool, error) { return f.active, nil }
func (f *fakeRepository) FinishBuilding(_ context.Context, _ Job, stage string, _ realroutes.Result) error {
	f.stage = stage
	f.finished = true
	return nil
}
func TestOutcomeAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		r          realroutes.Result
		err        error
	}{
		{name: "candidates", want: "awaiting_schedules", r: realroutes.Result{Candidates: []realroutes.Candidate{{}}}},
		{name: "empty", want: "no_routes"},
		{name: "outage", want: "failed", r: realroutes.Result{ProviderFailures: 1}},
		{name: "timeout", want: "failed", err: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{active: true}
			w := Worker{Repository: repo, Build: func(context.Context, []byte) (realroutes.Result, error) { return tc.r, tc.err }, DBTimeout: time.Second, BuildTimeout: time.Second, Poll: time.Millisecond, Lease: time.Minute, MaxAttempts: 3}
			if _, err := w.Step(context.Background()); err != nil || repo.stage != tc.want {
				t.Fatal(repo.stage, err)
			}
		})
	}
	repo := &fakeRepository{active: false}
	stopped := make(chan struct{})
	w := Worker{Repository: repo, Build: func(ctx context.Context, _ []byte) (realroutes.Result, error) {
		<-ctx.Done()
		close(stopped)
		return realroutes.Result{}, ctx.Err()
	}, DBTimeout: time.Second, BuildTimeout: time.Second, Poll: time.Millisecond, Lease: time.Minute, MaxAttempts: 3}
	if _, err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("provider not stopped")
	}
	if repo.finished {
		t.Fatal("cancelled work committed")
	}
	stopped = make(chan struct{})
	repo.active = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.Step(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
