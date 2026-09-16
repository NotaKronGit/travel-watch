package routes

import "context"

// MaxExplorationLegs bounds even non-recommended paths. It is a resource limit,
// not a connection-time rule or a claim that longer journeys are impossible.
const MaxExplorationLegs = 12

// ExplorationPlanner enumerates structural paths, including those outside MVP
// recommendations. The default GraphPlanner and booking validation are unchanged.
type ExplorationPlanner struct{}

func (ExplorationPlanner) Plan(ctx context.Context, s Snapshot, q Query, l Limits) (Result, error) {
	if !validInput(q, l) {
		return Result{}, ErrInvalidInput
	}
	nodes, links, err := prepareMode(ctx, s, l, true)
	if err != nil {
		return Result{}, err
	}
	if nodes[q.OriginCityID].Kind != CityPoint || nodes[q.DestinationCityID].Kind != CityPoint {
		return Result{}, ErrInvalidInput
	}
	adjacent := map[string][]Link{}
	for _, link := range links {
		adjacent[link.From] = append(adjacent[link.From], link)
	}
	r := Result{Candidates: []Candidate{}}
	steps := 0
	var walk func(string, []Link, map[string]bool) error
	walk = func(id string, path []Link, seen map[string]bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if steps >= l.MaxSteps {
			r.LimitReached = true
			return nil
		}
		steps++
		if id == q.DestinationCityID {
			if len(path) < 3 {
				return nil
			}
			if len(r.Candidates) >= l.MaxCandidates {
				r.LimitReached = true
				return nil
			}
			r.Candidates = append(r.Candidates, Candidate{Legs: append([]Link(nil), path...)})
			return nil
		}
		if len(path) >= MaxExplorationLegs {
			if len(adjacent[id]) > 0 {
				r.LimitReached = true
			}
			return nil
		}
		for _, link := range adjacent[id] {
			if seen[link.To] || (nodes[link.To].Kind == CityPoint && link.To != q.DestinationCityID) {
				continue
			}
			seen[link.To] = true
			if err := walk(link.To, append(path, link), seen); err != nil {
				return err
			}
			delete(seen, link.To)
			if r.LimitReached && (steps >= l.MaxSteps || len(r.Candidates) >= l.MaxCandidates) {
				break
			}
		}
		return nil
	}
	if err = walk(q.OriginCityID, nil, map[string]bool{q.OriginCityID: true}); err != nil {
		return Result{}, err
	}
	return ValidateExploration(ctx, s, q, l, r)
}

// ValidateExploration still rejects invented links, discontinuities, cycles and
// wrong endpoints. Complexity rules become annotations, never time guarantees.
func ValidateExploration(ctx context.Context, s Snapshot, q Query, l Limits, r Result) (Result, error) {
	if !validInput(q, l) {
		return Result{}, ErrInvalidInput
	}
	nodes, links, err := prepareMode(ctx, s, l, true)
	if err != nil {
		return Result{}, err
	}
	if nodes[q.OriginCityID].Kind != CityPoint || nodes[q.DestinationCityID].Kind != CityPoint {
		return Result{}, ErrInvalidInput
	}
	known := map[Link]bool{}
	for _, link := range links {
		known[link] = true
	}
	if len(r.Candidates) > l.MaxCandidates {
		return Result{}, ErrInvalidPlan
	}
	seenPaths := map[string]bool{}
	out := make([]Candidate, 0, len(r.Candidates))
	for _, c := range r.Candidates {
		if err = ctx.Err(); err != nil {
			return Result{}, err
		}
		if len(c.Legs) < 3 || len(c.Legs) > MaxExplorationLegs || c.Legs[0].From != q.OriginCityID || c.Legs[len(c.Legs)-1].To != q.DestinationCityID {
			return Result{}, ErrInvalidPlan
		}
		visited := map[string]bool{q.OriginCityID: true}
		for i, leg := range c.Legs {
			if !known[leg] || visited[leg.To] || (i > 0 && c.Legs[i-1].To != leg.From) || (nodes[leg.To].Kind == CityPoint && i != len(c.Legs)-1) {
				return Result{}, ErrInvalidPlan
			}
			visited[leg.To] = true
		}
		key := candidateKey(c)
		if !seenPaths[key] {
			seenPaths[key] = true
			out = append(out, Candidate{Legs: append([]Link(nil), c.Legs...)})
		}
	}
	r.Candidates = out
	r.Source = s.Source
	r.Synthetic = s.Synthetic
	r.SourceIncomplete = r.SourceIncomplete || !s.Complete
	return r, nil
}

// RecommendationWarnings applies the current recommendation policy after hard
// structural validation. Empty warnings do not imply verified schedules.
func RecommendationWarnings(s Snapshot, q Query, c Candidate) []string {
	nodes := map[string]Node{}
	for _, n := range s.Nodes {
		nodes[n.ID] = n
	}
	mainLegs := 0
	localTrain := false
	for _, leg := range c.Legs {
		if leg.Mode == Flight || leg.Mode == Train {
			mainLegs++
		}
		if leg.Mode == Train && (nodes[leg.From].CityID == nodes[leg.To].CityID || nodes[leg.To].CityID == q.OriginCityID || nodes[leg.To].CityID == q.DestinationCityID) {
			localTrain = true
		}
	}
	warnings := []string{}
	if mainLegs > 2 {
		warnings = append(warnings, "Больше двух основных транспортных участков; такая сложность пока вне правил рекомендаций.")
	}
	if localTrain {
		warnings = append(warnings, "Местный поезд до аэропорта пока не учтён в правилах рекомендаций как отдельный вариант доступа.")
	}
	if len(warnings) == 0 && !validShape(c.Legs, q, nodes) {
		warnings = append(warnings, "Такое сочетание транспорта пока вне правил рекомендаций.")
	}
	return warnings
}
