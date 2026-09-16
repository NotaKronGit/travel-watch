package planning

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// SourceResult retains each provider's native format and provenance. No routes
// or intermediate nodes are passed from one source to another.
type SourceResult struct {
	Data       json.RawMessage
	Count      int
	Incomplete bool
	Outcome    string
}
type Source func(context.Context, []byte) (SourceResult, error)
type MultiRepository interface {
	ClaimBuilding(context.Context, time.Duration, int) (Job, bool, error)
	BuildingActive(context.Context, Job) (bool, error)
	FinishSource(context.Context, Job, string, SourceResult) error
}
type MultiWorker struct {
	Repository                           MultiRepository
	Sources                              map[string]Source
	Poll, DBTimeout, BuildTimeout, Lease time.Duration
	MaxAttempts                          int
}

func (w MultiWorker) Step(ctx context.Context) (bool, error) {
	db, cancel := context.WithTimeout(ctx, w.DBTimeout)
	j, found, err := w.Repository.ClaimBuilding(db, w.Lease, w.MaxAttempts)
	cancel()
	if err != nil || !found {
		return found, err
	}
	if len(j.Sources) < 1 || len(j.Sources) > 2 {
		return true, errors.New("invalid saved source membership")
	}
	work, stop := context.WithTimeout(ctx, w.BuildTimeout)
	defer stop()
	type outcome struct {
		id     string
		result SourceResult
	}
	done := make(chan outcome, len(j.Sources))
	for _, id := range j.Sources {
		go func(id string) {
			var r SourceResult
			var err error
			if source := w.Sources[id]; source != nil {
				r, err = source(work, append([]byte(nil), j.Payload...))
			} else {
				err = errors.New("source unavailable")
				r.Outcome = "unavailable"
			}
			if err != nil {
				if r.Outcome == "" {
					r.Outcome = "error"
				}
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(work.Err(), context.DeadlineExceeded) {
					r.Outcome = "timeout"
				}
				r.Incomplete = true
			}
			done <- outcome{id, r}
		}(id)
	}
	remaining := len(j.Sources)
	// Built-in sources honor cancellation; always join them before releasing a job.
	defer func() {
		stop()
		for remaining > 0 {
			<-done
			remaining--
		}
	}()
	ticker := time.NewTicker(w.Poll)
	defer ticker.Stop()
	for remaining > 0 {
		select {
		case out := <-done:
			remaining--
			if ctx.Err() != nil {
				return true, ctx.Err()
			}
			db, cancel := context.WithTimeout(ctx, w.DBTimeout)
			err = w.Repository.FinishSource(db, j, out.id, out.result)
			cancel()
			if err != nil {
				return true, err
			}
		case <-ticker.C:
			db, cancel := context.WithTimeout(ctx, w.DBTimeout)
			active, e := w.Repository.BuildingActive(db, j)
			cancel()
			if e != nil || !active {
				return true, e
			}
		case <-ctx.Done():
			return true, ctx.Err()
		}
	}
	return true, nil
}
func (w MultiWorker) Run(ctx context.Context) error {
	if w.Repository == nil || len(w.Sources) < 1 || len(w.Sources) > 2 || w.Poll <= 0 || w.DBTimeout <= 0 || w.BuildTimeout <= 0 || w.Lease <= w.BuildTimeout+2*w.DBTimeout || w.MaxAttempts < 1 {
		return errors.New("invalid multi-source worker")
	}
	for ctx.Err() == nil {
		found, err := w.Step(ctx)
		if err != nil {
			return err
		}
		if !found {
			timer := time.NewTimer(w.Poll)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
	}
	return ctx.Err()
}
