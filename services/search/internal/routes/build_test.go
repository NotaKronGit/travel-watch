package routes_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes/testsource"
)

var limits = routes.Limits{MaxNodes: 100, MaxLinks: 100, MaxSteps: 1000, MaxCandidates: 20}
var query = routes.Query{OriginCityID: testsource.Origin, DestinationCityID: testsource.Destination}

type source struct {
	snapshot routes.Snapshot
	err      error
}

func (s source) Load(ctx context.Context, _ routes.Query, _ routes.Limits) (routes.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return routes.Snapshot{}, err
	}
	return s.snapshot, s.err
}
func build(t *testing.T, s routes.Snapshot, l routes.Limits) routes.Result {
	t.Helper()
	r, err := routes.Build(context.Background(), source{snapshot: s}, query, l)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func without(s routes.Snapshot, p func(routes.Link) bool) routes.Snapshot {
	s.Links = slices.DeleteFunc(s.Links, p)
	return s
}

func TestIvanovo(t *testing.T) {
	r, err := routes.Build(context.Background(), testsource.Source{}, query, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete() || !r.Synthetic || len(r.Candidates) != 5 {
		t.Fatalf("unexpected result: %+v", r)
	}
	airports := map[string]bool{}
	for _, c := range r.Candidates {
		flights, trains := 0, 0
		if c.Legs[0].From != query.OriginCityID || c.Legs[len(c.Legs)-1].To != query.DestinationCityID {
			t.Fatal("wrong endpoints")
		}
		for i, l := range c.Legs {
			if i > 0 && c.Legs[i-1].To != l.From {
				t.Fatal("disconnected route")
			}
			if l.Mode == routes.Flight {
				flights++
				airports[l.From] = true
			}
			if l.Mode == routes.Train {
				trains++
			}
		}
		if flights < 1 || flights+trains > 2 {
			t.Fatal("MVP exceeded")
		}
	}
	for _, id := range []string{"test-iwa", "test-svo", "test-goj"} {
		if !airports[id] {
			t.Fatal("missing airport", id)
		}
	}
	t.Log("Synthetic candidates: local flight, train and flight connections via Moscow and Nizhny Novgorod")
}
func TestMissingConnections(t *testing.T) {
	cases := []struct {
		name    string
		remove  func(routes.Link) bool
		want    int
		missing bool
	}{
		{"no direct flight", func(l routes.Link) bool { return l.From == "test-iwa" && l.To == "test-arrival" }, 4, false},
		{"no station transfer", func(l routes.Link) bool { return l.From == "test-moscow-station" && l.To == "test-svo" }, 4, false},
		{"no flights in covered snapshot", func(l routes.Link) bool { return l.Mode == routes.Flight }, 0, false},
		{"no origin access", func(l routes.Link) bool { return l.From == query.OriginCityID }, 0, true},
		{"no destination access", func(l routes.Link) bool { return l.To == query.DestinationCityID }, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := build(t, without(testsource.Ivanovo(), tc.remove), limits)
			if len(r.Candidates) != tc.want || r.MissingMapping != tc.missing {
				t.Fatalf("%+v", r)
			}
		})
	}
	s := testsource.Ivanovo()
	s.Complete = false
	r := build(t, s, limits)
	if r.Complete() || !r.SourceIncomplete || len(r.Candidates) != 5 {
		t.Fatal("lost partial candidates or coverage")
	}
	s = routes.Snapshot{Source: "unknown"}
	r = build(t, s, limits)
	if !r.MissingMapping || r.Complete() {
		t.Fatal("unknown coverage treated as empty")
	}
}
func TestDeterminismAndLimits(t *testing.T) {
	s := testsource.Ivanovo()
	want := build(t, s, limits)
	s.Links = append(s.Links, s.Links...)
	slices.Reverse(s.Links)
	slices.Reverse(s.Nodes)
	got := build(t, s, limits)
	if !reflect.DeepEqual(want, got) {
		t.Fatal("duplicates or order changed result")
	}
	l := limits
	l.MaxCandidates = 2
	a := build(t, s, l)
	slices.Reverse(s.Links)
	b := build(t, s, l)
	if !a.LimitReached || len(a.Candidates) != 2 || !reflect.DeepEqual(a, b) {
		t.Fatal("candidate cap not deterministic")
	}
	l.MaxCandidates = 5
	if build(t, s, l).LimitReached {
		t.Fatal("exact candidate cap incorrectly truncated")
	}
	l = limits
	l.MaxSteps = 1
	r := build(t, s, l)
	if !r.LimitReached || r.Complete() {
		t.Fatal("step budget ignored")
	}
	l = limits
	l.MaxLinks = 1
	if _, err := routes.Build(context.Background(), source{snapshot: s}, query, l); !errors.Is(err, routes.ErrSnapshotTooLarge) {
		t.Fatal("input cap ignored", err)
	}
}
func TestMultipleAirportsAndSharedAirport(t *testing.T) {
	s := testsource.Ivanovo()
	s.Nodes = append(s.Nodes, routes.Node{ID: "test-arrival-2", CityID: testsource.Destination, Name: "Second airport", Timezone: "Asia/Bangkok", Kind: routes.Airport})
	s.Links = append(s.Links, routes.Link{From: "test-iwa", To: "test-arrival-2", Mode: routes.Flight}, routes.Link{From: "test-arrival-2", To: testsource.Destination, Mode: routes.Transfer})
	if len(build(t, s, limits).Candidates) != 6 {
		t.Fatal("second airport lost")
	}
	// The airport is physically in another city but explicitly serves the destination.
	s.Cities = append(s.Cities, routes.City{ID: "test-neighbor", Name: "Neighbor", Timezone: "Asia/Bangkok"})
	s.Nodes[len(s.Nodes)-1].CityID = "test-neighbor"
	if len(build(t, s, limits).Candidates) != 6 {
		t.Fatal("shared airport lost")
	}
}
func TestInvalidDataAndCancellation(t *testing.T) {
	s := testsource.Ivanovo()
	s.Links = append(s.Links, routes.Link{From: "missing", To: "test-iwa", Mode: routes.Transfer})
	if _, err := routes.Build(context.Background(), source{snapshot: s}, query, limits); !errors.Is(err, routes.ErrInvalidSnapshot) {
		t.Fatal("dangling link accepted", err)
	}
	fail := errors.New("source down")
	if _, err := routes.Build(context.Background(), source{err: fail}, query, limits); !errors.Is(err, fail) {
		t.Fatal("source error swallowed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := routes.Build(ctx, testsource.Source{}, query, limits); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	q := query
	q.DestinationCityID = q.OriginCityID
	if _, err := routes.Build(context.Background(), testsource.Source{}, q, limits); !errors.Is(err, routes.ErrInvalidInput) {
		t.Fatal(err)
	}
}
func TestNoCycles(t *testing.T) {
	s := testsource.Ivanovo()
	s.Links = append(s.Links, routes.Link{From: "test-arrival", To: query.OriginCityID, Mode: routes.Transfer}, routes.Link{From: query.OriginCityID, To: "test-arrival", Mode: routes.Transfer}, routes.Link{From: "test-arrival", To: "test-iwa", Mode: routes.Flight})
	r := build(t, s, limits)
	for _, c := range r.Candidates {
		seen := map[string]bool{c.Legs[0].From: true}
		for _, l := range c.Legs {
			if seen[l.To] {
				t.Fatal("cycle", c)
			}
			seen[l.To] = true
		}
	}
	if len(r.Candidates) != 5 {
		t.Fatal("unexpected cyclic candidate")
	}
}

func TestAirportChange(t *testing.T) {
	s := testsource.Ivanovo()
	s.Nodes = append(s.Nodes, routes.Node{ID: "test-moscow-arrival", CityID: "test-city-moscow", Name: "Test arrival airport", Timezone: "Europe/Moscow", Kind: routes.Airport})
	s.Links = append(s.Links, routes.Link{From: "test-iwa", To: "test-moscow-arrival", Mode: routes.Flight})
	if len(build(t, s, limits).Candidates) != 5 {
		t.Fatal("airport change inferred without transfer")
	}
	s.Links = append(s.Links, routes.Link{From: "test-moscow-arrival", To: "test-svo", Mode: routes.Transfer})
	r := build(t, s, limits)
	if len(r.Candidates) != 6 {
		t.Fatal("explicit airport change lost")
	}
	found := false
	for _, c := range r.Candidates {
		if len(c.Legs) == 5 && c.Legs[1].Mode == routes.Flight && c.Legs[2].From == "test-moscow-arrival" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing flight-transfer-flight candidate")
	}
	// The same candidate must also pass the shared validator for an alternate planner.
	alternative := plannerFunc(func(context.Context, routes.Snapshot, routes.Query, routes.Limits) (routes.Result, error) {
		return r, nil
	})
	if _, err := (routes.Engine{Source: source{snapshot: s}, Planner: alternative}).Build(context.Background(), query, limits); err != nil {
		t.Fatal(err)
	}
	s = without(s, func(l routes.Link) bool { return l.From == "test-moscow-arrival" && l.To == "test-svo" })
	if _, err := (routes.Engine{Source: source{snapshot: s}, Planner: alternative}).Build(context.Background(), query, limits); !errors.Is(err, routes.ErrInvalidPlan) {
		t.Fatal("alternate planner invented airport transfer", err)
	}
}

func TestNoThirdFlightOrEndpointDetour(t *testing.T) {
	s := testsource.Ivanovo()
	s.Links = append(s.Links, routes.Link{From: "test-svo", To: "test-goj", Mode: routes.Flight})
	if len(build(t, s, limits).Candidates) != 5 {
		t.Fatal("third main leg allowed")
	}
	s.Nodes = append(s.Nodes, routes.Node{ID: "test-origin-other-airport", CityID: testsource.Origin, Name: "Origin second airport", Timezone: "Europe/Moscow", Kind: routes.Airport})
	s.Links = append(s.Links, routes.Link{From: "test-svo", To: "test-origin-other-airport", Mode: routes.Flight}, routes.Link{From: "test-origin-other-airport", To: testsource.Destination, Mode: routes.Transfer})
	r := build(t, s, limits)
	for _, c := range r.Candidates {
		if len(c.Legs) > 3 && c.Legs[1].To == "test-origin-other-airport" {
			t.Fatal("detour through origin city")
		}
	}
}
