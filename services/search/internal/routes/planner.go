package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Planner receives the same bounded topology for graph and future LLM approaches.
// It must honor context cancellation. Results are validated outside the planner.
type Planner interface {
	Plan(context.Context, Snapshot, Query, Limits) (Result, error)
}
type Engine struct {
	Source  Source
	Planner Planner
}

var ErrInvalidPlan = errors.New("planner returned invalid candidates")

func validInput(q Query, l Limits) bool {
	return q.OriginCityID != "" && q.DestinationCityID != "" && q.OriginCityID != q.DestinationCityID && l.MaxNodes > 0 && l.MaxLinks > 0 && l.MaxSteps > 0 && l.MaxCandidates > 0
}
func load(ctx context.Context, source Source, q Query, l Limits) (Snapshot, error) {
	if source == nil || !validInput(q, l) {
		return Snapshot{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	s, err := source.Load(ctx, q, l)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load transport snapshot: %w", err)
	}
	if _, _, err = prepare(ctx, s, l); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}
func (e Engine) Build(ctx context.Context, q Query, l Limits) (Result, error) {
	if e.Planner == nil {
		return Result{}, ErrInvalidInput
	}
	s, err := load(ctx, e.Source, q, l)
	if err != nil {
		return Result{}, err
	}
	return run(ctx, e.Planner, s, q, l)
}
func cloneSnapshot(s Snapshot) Snapshot {
	s.Cities = slices.Clone(s.Cities)
	s.Nodes = slices.Clone(s.Nodes)
	s.Links = slices.Clone(s.Links)
	return s
}
func run(ctx context.Context, p Planner, s Snapshot, q Query, l Limits) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	r, err := p.Plan(ctx, cloneSnapshot(s), q, l)
	if err != nil {
		return Result{}, err
	}
	if err = ctx.Err(); err != nil {
		return Result{}, err
	}
	return ValidateResult(ctx, s, q, l, r)
}

// ValidateResult applies the same topology rules to planner and schedule inputs.
func ValidateResult(ctx context.Context, s Snapshot, q Query, l Limits, r Result) (Result, error) {
	if !validInput(q, l) {
		return Result{}, ErrInvalidInput
	}
	if _, _, err := prepare(ctx, s, l); err != nil {
		return Result{}, err
	}
	var err error
	if len(r.Candidates) > l.MaxCandidates {
		return Result{}, ErrInvalidPlan
	}
	links := make(map[Link]bool)
	for _, link := range s.Links {
		links[link] = true
	}
	nodes := make(map[string]Node)
	for _, node := range s.Nodes {
		nodes[node.ID] = node
	}
	unique := make(map[string]bool)
	candidates := make([]Candidate, 0, len(r.Candidates))
	for _, c := range r.Candidates {
		if err = ctx.Err(); err != nil {
			return Result{}, err
		}
		if !validShape(c.Legs, q, nodes) || c.Legs[0].From != q.OriginCityID || c.Legs[len(c.Legs)-1].To != q.DestinationCityID {
			return Result{}, ErrInvalidPlan
		}

		seen := map[string]bool{q.OriginCityID: true}
		for i, leg := range c.Legs {
			if !links[leg] || seen[leg.To] || i > 0 && c.Legs[i-1].To != leg.From {
				return Result{}, ErrInvalidPlan
			}
			if leg.Mode == Train && (nodes[leg.To].CityID == q.OriginCityID || nodes[leg.To].CityID == q.DestinationCityID) {
				return Result{}, ErrInvalidPlan
			}
			seen[leg.To] = true
		}
		key := candidateKey(c)
		if !unique[key] {
			unique[key] = true
			candidates = append(candidates, Candidate{Legs: slices.Clone(c.Legs)})
		}
	}
	r.Candidates = candidates
	// Provenance and missing access cannot be overridden by a planner.
	r.Source, r.Synthetic = s.Source, s.Synthetic
	r.SourceIncomplete = r.SourceIncomplete || !s.Complete
	cities := make(map[string]bool)
	for _, city := range s.Cities {
		cities[city.ID] = true
	}
	origin, destination := false, false
	for _, link := range s.Links {
		origin = origin || link.Mode == Transfer && link.From == q.OriginCityID
		destination = destination || link.Mode == Transfer && link.To == q.DestinationCityID && nodes[link.From].Kind == Airport
	}
	r.MissingMapping = r.MissingMapping || !origin || !destination || !cities[q.OriginCityID] || !cities[q.DestinationCityID]
	return r, nil
}
func candidateKey(c Candidate) string { b, _ := json.Marshal(c.Legs); return string(b) }

type Attempt struct {
	Result   Result
	Err      error
	Duration time.Duration
}
type Comparison struct {
	Left, Right Attempt
	// Compared is false if either attempt failed: failure is not an empty result.
	Compared                    bool
	Common, OnlyLeft, OnlyRight []Candidate
}

// Compare loads once and executes sequentially with a separate timeout per planner.
// Each receives its own copy. No automatic merge, fallback or winner is selected.
func Compare(ctx context.Context, source Source, left, right Planner, q Query, l Limits, timeout time.Duration) (Comparison, error) {
	if left == nil || right == nil || timeout <= 0 {
		return Comparison{}, ErrInvalidInput
	}
	loadctx, cancel := context.WithTimeout(ctx, timeout)
	s, err := load(loadctx, source, q, l)
	cancel()
	if err != nil {
		return Comparison{}, err
	}
	attempt := func(p Planner) Attempt {
		planctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		start := time.Now()
		r, err := run(planctx, p, s, q, l)
		return Attempt{Result: r, Err: err, Duration: time.Since(start)}
	}
	comparison := Comparison{Left: attempt(left), Right: attempt(right)}
	if ctx.Err() != nil {
		return comparison, ctx.Err()
	}
	if comparison.Left.Err != nil || comparison.Right.Err != nil {
		return comparison, nil //nolint:nilerr // Per-planner failures are preserved in Attempt.Err; the comparison itself completed.
	}
	comparison.Compared = true
	rightSet := make(map[string]bool)
	leftSet := make(map[string]bool)
	for _, c := range comparison.Right.Result.Candidates {
		rightSet[candidateKey(c)] = true
	}
	for _, c := range comparison.Left.Result.Candidates {
		key := candidateKey(c)
		leftSet[key] = true
		if rightSet[key] {
			comparison.Common = append(comparison.Common, c)
		} else {
			comparison.OnlyLeft = append(comparison.OnlyLeft, c)
		}
	}
	for _, c := range comparison.Right.Result.Candidates {
		if !leftSet[candidateKey(c)] {
			comparison.OnlyRight = append(comparison.OnlyRight, c)
		}
	}
	return comparison, nil
}
