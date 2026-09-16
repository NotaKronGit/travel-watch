package planning

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type multiRepo struct {
	mu      sync.Mutex
	results map[string]SourceResult
	saved   chan string
}

func (r *multiRepo) ClaimBuilding(context.Context, time.Duration, int) (Job, bool, error) {
	return Job{Sources: []string{"graph", "gemini"}, Payload: []byte("origin and destination")}, true, nil
}
func (r *multiRepo) BuildingActive(context.Context, Job) (bool, error) { return true, nil }
func (r *multiRepo) FinishSource(_ context.Context, _ Job, id string, result SourceResult) error {
	r.mu.Lock()
	r.results[id] = result
	r.mu.Unlock()
	r.saved <- id
	return nil
}
func TestIndependentSourcesWaitAndPreservePartialResult(t *testing.T) {
	r := &multiRepo{results: map[string]SourceResult{}, saved: make(chan string, 2)}
	graphStarted := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	w := MultiWorker{Repository: r, DBTimeout: time.Second, BuildTimeout: time.Second, Lease: time.Minute, Poll: time.Second, MaxAttempts: 3, Sources: map[string]Source{
		"graph": func(ctx context.Context, payload []byte) (SourceResult, error) {
			close(graphStarted)
			select {
			case <-release:
				return SourceResult{Count: 2}, nil
			case <-ctx.Done():
				return SourceResult{}, ctx.Err()
			}
		},
		"gemini": func(ctx context.Context, payload []byte) (SourceResult, error) {
			select {
			case <-graphStarted:
			case <-ctx.Done():
				return SourceResult{}, ctx.Err()
			}
			return SourceResult{}, context.DeadlineExceeded
		},
	}}
	done := make(chan error, 1)
	go func() { _, err := w.Step(ctx); done <- err }()
	select {
	case id := <-r.saved:
		if id != "gemini" {
			t.Fatal(id)
		}
	case <-ctx.Done():
		t.Fatal("sources were not concurrent")
	}
	select {
	case <-done:
		t.Fatal("completed before graph result")
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.results["graph"].Count != 2 || r.results["gemini"].Outcome != "timeout" || !r.results["gemini"].Incomplete {
		t.Fatal(r.results)
	}
}
func TestMultiWorkerCancellationJoinsSources(t *testing.T) {
	r := &multiRepo{results: map[string]SourceResult{}, saved: make(chan string, 2)}
	started := make(chan struct{}, 2)
	stopped := make(chan struct{}, 2)
	source := func(ctx context.Context, _ []byte) (SourceResult, error) {
		started <- struct{}{}
		<-ctx.Done()
		stopped <- struct{}{}
		return SourceResult{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := MultiWorker{Repository: r, Sources: map[string]Source{"graph": source, "gemini": source}, DBTimeout: time.Second, BuildTimeout: time.Minute, Poll: time.Second}
	done := make(chan error, 1)
	go func() { _, err := w.Step(ctx); done <- err }()
	<-started
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(stopped) != 2 || len(r.results) != 0 {
		t.Fatal("source escaped cancellation or result was saved")
	}
}
