package planning

import (
	"context"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"time"
)

type Worker struct {
	Repository                           Repository
	Build                                func(context.Context, []byte) (realroutes.Result, error)
	Poll, DBTimeout, BuildTimeout, Lease time.Duration
	MaxAttempts                          int
}

func (w Worker) Step(ctx context.Context) (bool, error) {
	db, cancel := context.WithTimeout(ctx, w.DBTimeout)
	j, found, err := w.Repository.ClaimBuilding(db, w.Lease, w.MaxAttempts)
	cancel()
	if err != nil || !found {
		return false, err
	}
	work, stop := context.WithTimeout(ctx, w.BuildTimeout)
	defer stop()
	type outcome struct {
		result realroutes.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() { r, e := w.Build(work, j.Payload); done <- outcome{r, e} }()
	ticker := time.NewTicker(w.Poll)
	defer ticker.Stop()
	for {
		select {
		case out := <-done:
			if ctx.Err() != nil {
				return true, ctx.Err()
			}
			stage := "awaiting_schedules"
			if out.err != nil || (len(out.result.Candidates) == 0 && out.result.ProviderFailures > 0) {
				stage = "failed"
			} else if len(out.result.Candidates) == 0 {
				stage = "no_routes"
			}
			db, cancel := context.WithTimeout(ctx, w.DBTimeout)
			defer cancel()
			return true, w.Repository.FinishBuilding(db, j, stage, out.result)
		case <-ticker.C:
			db, cancel := context.WithTimeout(ctx, w.DBTimeout)
			active, e := w.Repository.BuildingActive(db, j)
			cancel()
			if e != nil || !active {
				stop()
				<-done
				return true, e
			}
		case <-ctx.Done():
			stop()
			<-done
			return true, ctx.Err()
		}
	}
}
func (w Worker) Run(ctx context.Context) error {
	if w.Repository == nil || w.Build == nil || w.Poll <= 0 || w.DBTimeout <= 0 || w.BuildTimeout <= 0 || w.Lease <= w.BuildTimeout+2*w.DBTimeout || w.MaxAttempts < 1 {
		return errors.New("invalid route worker")
	}
	for ctx.Err() == nil {
		found, err := w.Step(ctx)
		if err != nil {
			return err
		}
		if !found {
			t := time.NewTimer(w.Poll)
			select {
			case <-ctx.Done():
				t.Stop()
			case <-t.C:
			}
		}
	}
	return ctx.Err()
}
