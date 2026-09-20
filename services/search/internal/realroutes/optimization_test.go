package realroutes

import (
	"context"
	"fmt"
	"github.com/NotaKronGit/travel-watch/api/transport"
	"reflect"
	"testing"

	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
)

func TestDiscoveryVisitsPairsBeforeRepeatingDates(t *testing.T) {
	p, q := fixture()
	p.Config.GoogleFlights = FlightConfig{Enabled: true, MaxRequests: 3, MaxOptions: 20}
	f := &flightFixture{}
	p.Provider = f
	q.DepartureFrom, q.DepartureTo, q.Adults = "2027-01-01", "2027-01-07", 1
	result, err := p.Plan(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, req := range f.requests {
		key := req.From + "/" + req.To
		if seen[key] || req.Date != "2027-01-01" {
			t.Fatalf("pair monopolizes budget: %+v", f.requests)
		}
		seen[key] = true
	}
	if len(seen) != 3 || !result.LimitReached {
		t.Fatalf("missing breadth: %+v", result)
	}
}

func TestNearPrioritizesLargeBeforeTruncation(t *testing.T) {
	p, q := fixture()
	p.Config.MaxAirports = 1
	p.Airports = append(p.Airports, airports.Airport{SourceID: 20, IATA: "SML", Longitude: 20, Type: "medium_airport", Scheduled: true})
	p.Airports[3].Longitude = 20.1
	s := run{p: p}
	got := s.near(q.Destination, p.Config.AirportRadiusKM)
	if len(got) != 1 || got[0].IATA != "DST" {
		t.Fatalf("large airport displaced: %+v", got)
	}
}

func railSteps(station, via string) []Step {
	return []Step{
		{From: "City", To: "Origin rail", Mode: "transfer", Evidence: "assumed"},
		{From: "Origin rail", To: station, FromCode: "origin-rail", ToCode: station, Mode: "train", Evidence: "yandex-rasp"},
		{From: station, To: "Hub", Mode: "transfer", Evidence: "assumed"},
		{From: "Hub", To: via, FromCode: "iata:HUB", ToCode: "iata:" + via, Mode: "plane", Evidence: "google-flights-fli"},
		{From: via, To: "Destination airport", FromCode: "iata:" + via, ToCode: "iata:DST", Mode: "plane", Evidence: "google-flights-fli"},
		{From: "Destination airport", To: "Destination", Mode: "transfer", Evidence: "assumed"},
	}
}
func TestRailAlternativesDoNotConsumeSchemeBudget(t *testing.T) {
	p, _ := fixture()
	p.Config.MaxCandidates = 2
	s := run{p: p, seen: map[string]bool{}}
	for i := 0; i < 4; i++ {
		s.add(railSteps(fmt.Sprint(i), "AAA"))
	}
	s.add(railSteps("0", "BBB"))
	if len(s.r.Candidates) != 2 || s.r.Candidates[1].Steps[3].To != "BBB" {
		t.Fatalf("stations consumed scheme budget: %+v", s.r.Candidates)
	}
}

func TestRailVariantsBoundedAndLateAccessRetained(t *testing.T) {
	p, _ := fixture()
	p.Config.MaxCandidates = 1
	p.Config.MaxRailAccessVariants = 2
	s := run{p: p}
	s.add(railSteps("a", "AAA"))
	s.add(railSteps("a", "BBB")) // Unique-scheme cap reached.
	s.add(railSteps("b", "AAA")) // Still retain access for an existing group.
	s.add(railSteps("b", "AAA")) // Exact duplicate does not consume access budget.
	s.add(railSteps("c", "AAA"))
	if len(s.r.Candidates) != 1 || len(s.r.Candidates[0].RailAccessVariants) != 2 || !s.r.LimitReached {
		t.Fatalf("wrong independent bounds: %+v", s.r)
	}
	if s.r.Candidates[0].RailAccessVariants[1][1].ToCode != "b" {
		t.Fatal("station identity lost")
	}
	for _, n := range []int{-1, 21} {
		p.Config.MaxRailAccessVariants = n
		if p.Config.Validate() == nil {
			t.Fatal("invalid limit accepted")
		}
	}
}

func TestAirportMunicipalitiesInterleavedBeforeHubCap(t *testing.T) {
	list := []airports.Airport{
		{IATA: "AAA", Municipality: "Metro", Country: "AA", Type: "large_airport", Longitude: 1},
		{IATA: "AAB", Municipality: "Metro", Country: "AA", Type: "large_airport", Longitude: 1.1},
		{IATA: "BBB", Municipality: "Elsewhere", Country: "AA", Type: "large_airport", Longitude: 2},
		{IATA: "AAA", Municipality: "Metro", Country: "AA", Type: "large_airport", Longitude: 1},
	}
	got := diverseAirports(list, transport.Point{})
	if len(got) != 3 || got[0].IATA != "AAA" || got[1].IATA != "BBB" || got[2].IATA != "AAB" {
		t.Fatalf("municipality monopolizes budget: %+v", got)
	}
	if list[1].IATA != "AAB" {
		t.Fatal("input mutated")
	}
}

func TestExactFlightBudgetDoesNotClaimTruncation(t *testing.T) {
	p, q := fixture()
	f := &flightFixture{}
	p.Provider = f
	p.Config.GoogleFlights = FlightConfig{Enabled: true, MaxRequests: 1, MaxOptions: 20}
	q.DepartureFrom, q.DepartureTo, q.Adults = "2027-01-01", "2027-01-01", 1
	s := run{p: p, cache: map[string]transport.Response{}, failures: map[string]bool{}}
	s.googlePaths(context.Background(), q, p.Airports[:1], p.Airports[3:], nil, "")
	if len(f.requests) != 1 || s.r.LimitReached {
		t.Fatalf("exact coverage marked truncated: %+v", s.r)
	}
}

func TestEmptyFirstDateDoesNotExcludeLaterObservation(t *testing.T) {
	p, q := fixture()
	q.DepartureFrom, q.DepartureTo, q.Adults = "2027-01-01", "2027-01-02", 1
	f := &flightFixture{}
	p.Provider = providerFunc(func(ctx context.Context, req transport.Request) (transport.Response, error) {
		if req.Method == "flight-paths" && req.Date == q.DepartureFrom {
			return transport.Response{}, nil
		}
		return f.Call(ctx, req)
	})
	p.Config.GoogleFlights = FlightConfig{Enabled: true, MaxRequests: 2, MaxOptions: 20}
	s := run{p: p, cache: map[string]transport.Response{}, failures: map[string]bool{}}
	s.googlePaths(context.Background(), q, p.Airports[:1], p.Airports[3:], nil, "")
	if len(s.r.Candidates) != 1 || !reflect.DeepEqual(s.r.Candidates[0].Steps[1].ObservedDates, []string{q.DepartureTo}) {
		t.Fatalf("later observation lost: %+v", s.r)
	}
}

func TestRailGroupingPreservesAirportChangesAndEvidence(t *testing.T) {
	a := railSteps("a", "AAA")
	b := railSteps("b", "AAA")
	b[3].Evidence = "another-source"
	if RailAccessKey(Candidate{Steps: a}) == RailAccessKey(Candidate{Steps: b}) {
		t.Fatal("different evidence merged")
	}
	b = railSteps("b", "AAA")
	b[4].From = "Other airport"
	b[4].FromCode = "iata:OTH"
	b = append(b[:4], append([]Step{{From: "AAA", To: "Other airport", Mode: "transfer", Evidence: "assumed"}}, b[4:]...)...)
	if RailAccessKey(Candidate{Steps: b}) == "" || RailAccessKey(Candidate{Steps: a}) == RailAccessKey(Candidate{Steps: b}) {
		t.Fatal("airport change lost")
	}
}
