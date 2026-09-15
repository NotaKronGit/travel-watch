package routes_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes/testsource"
)

type plannerFunc func(context.Context, routes.Snapshot, routes.Query, routes.Limits) (routes.Result, error)

func (f plannerFunc) Plan(ctx context.Context, s routes.Snapshot, q routes.Query, l routes.Limits) (routes.Result, error) {
	return f(ctx, s, q, l)
}

type countedSource struct{ calls int }

func (s *countedSource) Load(context.Context, routes.Query, routes.Limits) (routes.Snapshot, error) {
	s.calls++
	return testsource.Ivanovo(), nil
}
func TestPlannerComparison(t *testing.T) {
	src := &countedSource{}
	alternate := plannerFunc(func(ctx context.Context, s routes.Snapshot, q routes.Query, l routes.Limits) (routes.Result, error) {
		r, err := (routes.GraphPlanner{}).Plan(ctx, s, q, l)
		if err != nil {
			return r, err
		}
		r.Candidates = r.Candidates[:1]
		r.SourceIncomplete = true
		return r, nil
	})
	c, err := routes.Compare(context.Background(), src, routes.GraphPlanner{}, alternate, query, limits, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if src.calls != 1 || !c.Compared || len(c.Common) != 1 || len(c.OnlyLeft) != 4 || len(c.OnlyRight) != 0 || !c.Right.Result.SourceIncomplete {
		t.Fatalf("comparison: %+v", c)
	}
	r, err := (routes.Engine{Source: src, Planner: alternate}).Build(context.Background(), query, limits)
	if err != nil || len(r.Candidates) != 1 {
		t.Fatal("switch planner", err)
	}
}
func TestInvalidPlannerResults(t *testing.T) {
	for _, kind := range []string{"invented link", "disconnected", "three flights", "oversized", "empty candidate"} {
		t.Run(kind, func(t *testing.T) {
			bad := plannerFunc(func(ctx context.Context, s routes.Snapshot, q routes.Query, l routes.Limits) (routes.Result, error) {
				r, err := (routes.GraphPlanner{}).Plan(ctx, s, q, l)
				if err != nil {
					return r, err
				}
				switch kind {
				case "invented link":
					r.Candidates[0].Legs[1].To = "imaginary-airport"
				case "disconnected":
					r.Candidates[0].Legs[0] = r.Candidates[1].Legs[0]
				case "three flights":
					r.Candidates[0].Legs = []routes.Link{{From: testsource.Origin, To: "test-iwa", Mode: routes.Transfer}, {From: "test-iwa", To: "test-svo", Mode: routes.Flight}, {From: "test-svo", To: "test-goj", Mode: routes.Flight}, {From: "test-goj", To: "test-arrival", Mode: routes.Flight}, {From: "test-arrival", To: testsource.Destination, Mode: routes.Transfer}}
				case "oversized":
					for len(r.Candidates) <= l.MaxCandidates {
						r.Candidates = append(r.Candidates, r.Candidates[0])
					}
				case "empty candidate":
					r.Candidates = []routes.Candidate{{}}
				}
				return r, nil
			})
			c, err := routes.Compare(context.Background(), testsource.Source{}, bad, routes.GraphPlanner{}, query, limits, time.Second)
			if err != nil || c.Compared || !errors.Is(c.Left.Err, routes.ErrInvalidPlan) || c.Right.Err != nil || len(c.Right.Result.Candidates) != 5 {
				t.Fatalf("invalid result accepted: %+v %v", c, err)
			}
		})
	}
}
func TestPlannerIsolationAndFailure(t *testing.T) {
	fail := errors.New("test planner failure")
	mutating := plannerFunc(func(_ context.Context, s routes.Snapshot, _ routes.Query, _ routes.Limits) (routes.Result, error) {
		s.Links[0].To = "corrupted"
		s.Nodes[0].ID = "corrupted"
		s.Cities[0].ID = "corrupted"
		return routes.Result{}, fail
	})
	c, err := routes.Compare(context.Background(), testsource.Source{}, mutating, routes.GraphPlanner{}, query, limits, time.Second)
	if err != nil || c.Compared || !errors.Is(c.Left.Err, fail) || c.Right.Err != nil || len(c.Right.Result.Candidates) != 5 {
		t.Fatal("snapshot isolation failed", c, err)
	}
	waiting := plannerFunc(func(ctx context.Context, _ routes.Snapshot, _ routes.Query, _ routes.Limits) (routes.Result, error) {
		<-ctx.Done()
		return routes.Result{}, ctx.Err()
	})
	c, err = routes.Compare(context.Background(), testsource.Source{}, waiting, routes.GraphPlanner{}, query, limits, 50*time.Millisecond)
	if err != nil || !errors.Is(c.Left.Err, context.DeadlineExceeded) || c.Right.Err != nil {
		t.Fatal("independent timeout", c, err)
	}
}
func TestPlannerProvenanceAndDedup(t *testing.T) {
	s := testsource.Ivanovo()
	s.Complete = false
	p := plannerFunc(func(ctx context.Context, s routes.Snapshot, q routes.Query, l routes.Limits) (routes.Result, error) {
		r, err := (routes.GraphPlanner{}).Plan(ctx, s, q, l)
		if err != nil {
			return r, err
		}
		r.Source = "invented"
		r.Synthetic = false
		r.SourceIncomplete = false
		r.Candidates = append(r.Candidates, r.Candidates[0])
		return r, nil
	})
	r, err := (routes.Engine{Source: source{snapshot: s}, Planner: p}).Build(context.Background(), query, limits)
	if err != nil || len(r.Candidates) != 5 || !r.Synthetic || !r.SourceIncomplete || r.Source != s.Source {
		t.Fatal("provenance or dedup lost", r, err)
	}
}
