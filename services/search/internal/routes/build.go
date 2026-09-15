package routes

import (
	"cmp"
	"context"
	"errors"
	"slices"
)

var (
	ErrInvalidInput     = errors.New("invalid route query or limits")
	ErrInvalidSnapshot  = errors.New("invalid transport snapshot")
	ErrSnapshotTooLarge = errors.New("transport snapshot exceeds input limits")
)

type incomingKey struct {
	to   string
	mode Mode
}

// Build retains the default deterministic planner for simple callers.
func Build(ctx context.Context, source Source, q Query, limits Limits) (Result, error) {
	return (Engine{Source: source, Planner: GraphPlanner{}}).Build(ctx, q, limits)
}

// GraphPlanner walks backwards using the MVP patterns.
type GraphPlanner struct{}

func (GraphPlanner) Plan(ctx context.Context, snapshot Snapshot, q Query, limits Limits) (Result, error) {
	if !validInput(q, limits) {
		return Result{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	nodes, links, err := prepare(ctx, snapshot, limits)
	if err != nil {
		return Result{}, err
	}
	result := Result{Source: snapshot.Source, Synthetic: snapshot.Synthetic, SourceIncomplete: !snapshot.Complete, Candidates: []Candidate{}}
	if nodes[q.OriginCityID].Kind != CityPoint || nodes[q.DestinationCityID].Kind != CityPoint {
		result.MissingMapping = true
		return result, nil
	}
	incoming := make(map[incomingKey][]Link)
	originMapped, destinationMapped := false, false
	for _, link := range links {
		incoming[incomingKey{link.To, link.Mode}] = append(incoming[incomingKey{link.To, link.Mode}], link)
		originMapped = originMapped || link.Mode == Transfer && link.From == q.OriginCityID
		destinationMapped = destinationMapped || link.Mode == Transfer && link.To == q.DestinationCityID && nodes[link.From].Kind == Airport
	}
	if !originMapped || !destinationMapped {
		result.MissingMapping = true
		return result, nil
	}
	// Traverse each allowed pattern backwards from the destination.
	steps := 0
	var walk func(string, []Mode, []Link, map[string]bool) error
	walk = func(to string, remaining []Mode, path []Link, seen map[string]bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(remaining) == 0 {
			if to != q.OriginCityID {
				return nil
			}
			legs := slices.Clone(path)
			slices.Reverse(legs)
			if !validShape(legs, q, nodes) {
				return nil
			}
			if len(result.Candidates) == limits.MaxCandidates {
				result.LimitReached = true
				return nil
			}
			result.Candidates = append(result.Candidates, Candidate{Legs: legs})
			return nil
		}
		for _, link := range incoming[incomingKey{to, remaining[0]}] {
			if err := ctx.Err(); err != nil {
				return err
			}
			if result.LimitReached {
				return nil
			}
			if steps == limits.MaxSteps {
				result.LimitReached = true
				return nil
			}
			steps++
			if seen[link.From] {
				continue
			}
			if link.Mode == Train && (nodes[link.To].CityID == q.OriginCityID || nodes[link.To].CityID == q.DestinationCityID) {
				continue
			}
			seen[link.From] = true
			if err := walk(link.From, remaining[1:], append(path, link), seen); err != nil {
				return err
			}
			delete(seen, link.From)
		}
		return nil
	}
	for _, pattern := range patterns() {
		slices.Reverse(pattern)
		if result.LimitReached {
			break
		}
		if err := walk(q.DestinationCityID, pattern, nil, map[string]bool{q.DestinationCityID: true}); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func prepare(ctx context.Context, s Snapshot, limits Limits) (map[string]Node, []Link, error) {
	if len(s.Cities) > limits.MaxNodes || len(s.Nodes) > limits.MaxNodes-len(s.Cities) || len(s.Links) > limits.MaxLinks {
		return nil, nil, ErrSnapshotTooLarge
	}
	if s.Source == "" {
		return nil, nil, ErrInvalidSnapshot
	}
	nodes := make(map[string]Node)
	for _, city := range s.Cities {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if city.ID == "" || city.Name == "" || city.Timezone == "" || nodes[city.ID].ID != "" {
			return nil, nil, ErrInvalidSnapshot
		}
		nodes[city.ID] = Node{ID: city.ID, CityID: city.ID, Name: city.Name, Timezone: city.Timezone, Kind: CityPoint}
	}
	for _, node := range s.Nodes {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if node.ID == "" || node.Name == "" || node.Timezone == "" || nodes[node.ID].ID != "" || nodes[node.CityID].Kind != CityPoint || (node.Kind != Airport && node.Kind != Station) {
			return nil, nil, ErrInvalidSnapshot
		}
		nodes[node.ID] = node
	}
	links := slices.Clone(s.Links)
	for _, link := range links {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		from, to := nodes[link.From], nodes[link.To]
		if from.ID == "" || to.ID == "" || from.ID == to.ID {
			return nil, nil, ErrInvalidSnapshot
		}
		valid := false
		switch link.Mode {
		case Flight:
			valid = from.Kind == Airport && to.Kind == Airport && from.CityID != to.CityID
		case Train:
			valid = from.Kind == Station && to.Kind == Station && from.CityID != to.CityID
		case Transfer:
			valid = from.Kind == CityPoint && (to.Kind == Airport || to.Kind == Station) ||
				to.Kind == CityPoint && (from.Kind == Airport || from.Kind == Station) ||
				(from.Kind == Station || from.Kind == Airport) && to.Kind == Airport && from.CityID == to.CityID
		}
		if !valid {
			return nil, nil, ErrInvalidSnapshot
		}
	}
	slices.SortFunc(links, func(a, b Link) int {
		return cmp.Or(cmp.Compare(a.Mode, b.Mode), cmp.Compare(a.To, b.To), cmp.Compare(a.From, b.From))
	})
	return nodes, slices.Compact(links), nil
}
