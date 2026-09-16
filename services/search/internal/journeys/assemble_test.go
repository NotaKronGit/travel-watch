package journeys_test

import (
	"context"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	test "github.com/NotaKronGit/travel-watch/services/search/internal/journeys/testsource"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	routetest "github.com/NotaKronGit/travel-watch/services/search/internal/routes/testsource"
	"reflect"
	"slices"
	"testing"
	"time"
)

var limits = journeys.Limits{Routes: routes.Limits{MaxNodes: 100, MaxLinks: 100, MaxSteps: 1000, MaxCandidates: 20}, MaxOffers: 100, MaxTransfers: 100, MaxCombinations: 1000, MaxJourneys: 20}

func candidates(t *testing.T, top routes.Snapshot) routes.Result {
	t.Helper()
	r, err := (routes.GraphPlanner{}).Plan(context.Background(), top, test.Query().Route, limits.Routes)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func assemble(t *testing.T, s journeys.Snapshot, p journeys.Policy, q journeys.Query) journeys.Result {
	t.Helper()
	top := routetest.Ivanovo()
	r, err := journeys.Assemble(context.Background(), top, candidates(t, top), s, q, p, limits)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func hasOffer(j journeys.Journey, id string) bool {
	for _, leg := range j.Legs {
		if leg.Offer != nil && leg.Offer.ID == id {
			return true
		}
	}
	return false
}
func TestIvanovoSchedules(t *testing.T) {
	r := assemble(t, test.Snapshot(), test.Policy(), test.Query())
	if !r.Complete() || !r.Synthetic || len(r.Journeys) != 5 {
		t.Fatalf("result %+v", r)
	}
	counts := map[string]int{}
	for _, j := range r.Journeys {
		for _, leg := range j.Legs {
			if leg.Offer != nil {
				counts[leg.Offer.ID]++
			}
		}
		if hasOffer(j, "nizhny-too-early") {
			t.Fatal("impossible connection accepted")
		}
		if !j.End.After(j.Start) {
			t.Fatal("invalid interval")
		}
		for i, leg := range j.Legs {
			if i > 0 && leg.Start.Before(j.Legs[i-1].End) {
				t.Fatal("overlap")
			}
		}
	}
	for id, want := range map[string]int{"local-direct": 1, "moscow-flight": 2, "nizhny-next-day": 2, "nizhny-too-early": 0} {
		if counts[id] != want {
			t.Fatalf("%s: got %d want %d", id, counts[id], want)
		}
	}
	// Direct start includes access + boarding; end includes exit + final transfer.
	for _, j := range r.Journeys {
		if hasOffer(j, "local-direct") {
			if j.Start.Format(time.RFC3339) != "2027-01-10T10:30:00+03:00" || j.End.Format(time.RFC3339) != "2027-01-11T00:30:00+07:00" {
				t.Fatalf("boundaries %s %s", j.Start, j.End)
			}
		}
	}
	t.Log("5 valid trips; early Nizhny flights rejected, next-day flights retained")
}
func TestConnectionBoundary(t *testing.T) {
	for _, shift := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		s := test.Snapshot()
		// Moscow train: 10:00 + exit 10m + transfer 90m + reserve 30m + boarding 60m = 13:10.
		for i := range s.Offers {
			if s.Offers[i].ID == "moscow-flight" {
				s.Offers[i].Departure = time.Date(2027, 1, 10, 13, 10, 0, 0, time.FixedZone("", 3*3600)).Add(shift)
			}
		}
		r := assemble(t, s, test.Policy(), test.Query())
		found := false
		for _, j := range r.Journeys {
			if hasOffer(j, "train-moscow") {
				found = true
			}
		}
		if found != (shift >= 0) {
			t.Fatalf("boundary %s accepted=%v", shift, found)
		}
	}
}
func TestAirportChange(t *testing.T) {
	top := routetest.Ivanovo()
	top.Nodes = append(top.Nodes, routes.Node{ID: "test-other-moscow", CityID: "test-city-moscow", Name: "Different arrival airport", Timezone: "Europe/Moscow", Kind: routes.Airport})
	flight := routes.Link{From: "test-iwa", To: "test-other-moscow", Mode: routes.Flight}
	transfer := routes.Link{From: "test-other-moscow", To: "test-svo", Mode: routes.Transfer}
	top.Links = append(top.Links, flight, transfer)
	s := test.Snapshot()
	o := s.Offers[6]
	o.ID = "other-airport-feeder"
	o.Link = flight
	s.Offers = append(s.Offers, o)
	run := func() journeys.Result {
		t.Helper()
		r, err := journeys.Assemble(context.Background(), top, candidates(t, top), s, test.Query(), test.Policy(), limits)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if !run().MissingTiming {
		t.Fatal("unknown airport transfer ignored")
	}
	s.Transfers = append(s.Transfers, journeys.TransferTiming{Link: transfer, Source: s.Source, ObservedAt: o.ObservedAt, Known: true, Duration: 2 * time.Hour})
	r := run()
	if len(r.Journeys) != 6 || r.MissingTiming {
		t.Fatal("valid change rejected")
	}
	s.Transfers[len(s.Transfers)-1].Duration += time.Nanosecond
	if len(run().Journeys) != 5 {
		t.Fatal("insufficient airport transfer accepted")
	}
}
func TestDateWindowAndUnknownTiming(t *testing.T) {
	s := test.Snapshot()
	q := test.Query()
	q.DepartureTo = "2027-01-11"
	s.Offers[0].Departure = time.Date(2027, 1, 11, 0, 30, 0, 0, time.FixedZone("", 3*3600))
	s.Offers[0].Arrival = s.Offers[0].Departure.Add(7 * time.Hour)
	q.DepartureFrom = "2027-01-11"
	for _, j := range assemble(t, s, test.Policy(), q).Journeys {
		if hasOffer(j, "local-direct") {
			t.Fatal("trip starts previous day")
		}
	}
	q = test.Query()
	r := assemble(t, s, test.Policy(), q)
	found := false
	for _, j := range r.Journeys {
		found = found || hasOffer(j, "local-direct")
	}
	if !found {
		t.Fatal("next-day first flight incorrectly excluded")
	}
	s = test.Snapshot()
	s.Transfers[0].Known = false
	s.Transfers[0].Duration = 0
	r = assemble(t, s, test.Policy(), test.Query())
	if !r.MissingTiming || r.Complete() {
		t.Fatal("unknown treated as zero")
	}
	s = test.Snapshot()
	s.Complete = false
	if assemble(t, s, test.Policy(), test.Query()).Complete() {
		t.Fatal("partial source treated as complete")
	}
}
func TestLimitsAndInvalidData(t *testing.T) {
	top := routetest.Ivanovo()
	cs := candidates(t, top)
	s := test.Snapshot()
	l := limits
	l.MaxJourneys = 2
	r, err := journeys.Assemble(context.Background(), top, cs, s, test.Query(), test.Policy(), l)
	if err != nil || !r.LimitReached || len(r.Journeys) != 2 {
		t.Fatal("journey limit", err)
	}
	l.MaxJourneys = 5
	r, err = journeys.Assemble(context.Background(), top, cs, s, test.Query(), test.Policy(), l)
	if err != nil || r.LimitReached {
		t.Fatal("exact cap", err)
	}
	l = limits
	l.MaxCombinations = 1
	r, err = journeys.Assemble(context.Background(), top, cs, s, test.Query(), test.Policy(), l)
	if err != nil || !r.LimitReached {
		t.Fatal("work limit", err)
	}
	s.Offers = append(s.Offers, s.Offers[0])
	if _, err = journeys.Assemble(context.Background(), top, cs, s, test.Query(), test.Policy(), limits); !errors.Is(err, journeys.ErrInvalidInput) {
		t.Fatal("duplicate identity", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = journeys.Assemble(ctx, top, cs, test.Snapshot(), test.Query(), test.Policy(), limits); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestOrderPricesAndPassengers(t *testing.T) {
	s := test.Snapshot()
	s.Offers[0].Price = nil
	a := assemble(t, s, test.Policy(), test.Query())
	slices.Reverse(s.Offers)
	b := assemble(t, s, test.Policy(), test.Query())
	if !reflect.DeepEqual(a, b) {
		t.Fatal("source order changed result")
	}
	q := test.Query()
	q.Adults = 2
	if len(assemble(t, s, test.Policy(), q).Journeys) != 0 {
		t.Fatal("one-passenger quote used for two")
	}
	p := test.Policy()
	p.MaxConnection = 3 * time.Hour
	for _, j := range assemble(t, s, p, test.Query()).Journeys {
		if hasOffer(j, "nizhny-next-day") {
			t.Fatal("max wait ignored")
		}
	}
	p = test.Policy()
	p.MaxJourney = time.Hour
	if len(assemble(t, s, p, test.Query()).Journeys) != 0 {
		t.Fatal("max journey ignored")
	}
}

func TestLocalDateWithDSTAndUTCOffers(t *testing.T) {
	top := routetest.Ivanovo()
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for i := range top.Cities {
		if top.Cities[i].ID == routetest.Origin {
			top.Cities[i].Timezone = "Europe/Berlin"
		}
	}
	q := test.Query()
	q.DepartureFrom = "2027-03-28"
	q.DepartureTo = q.DepartureFrom
	// Berlin's spring transition produces a 23-hour calendar day, not 24 hours.
	nextMidnight := time.Date(2027, 3, 29, 0, 0, 0, 0, location)
	for _, delta := range []time.Duration{-time.Nanosecond, 0} {
		s := test.Snapshot()
		s.Offers = s.Offers[:1]
		s.Offers[0].Departure = nextMidnight.Add(delta).Add(90 * time.Minute).UTC()
		s.Offers[0].Arrival = s.Offers[0].Departure.Add(7 * time.Hour)
		r, err := journeys.Assemble(context.Background(), top, candidates(t, top), s, q, test.Policy(), limits)
		if err != nil {
			t.Fatal(err)
		}
		expected := 0
		if delta < 0 {
			expected = 1
		}
		if len(r.Journeys) != expected {
			t.Fatal("incorrect local day boundary", delta, len(r.Journeys))
		}
	}
}
func TestBadSchedulesAndUntrustedCandidate(t *testing.T) {
	for _, kind := range []string{"arrival", "currency", "transfer", "link"} {
		t.Run(kind, func(t *testing.T) {
			s := test.Snapshot()
			switch kind {
			case "arrival":
				s.Offers[0].Arrival = s.Offers[0].Departure
			case "currency":
				s.Offers[0].Price.Currency = "bad"
			case "transfer":
				s.Transfers[0].Duration = -time.Minute
			case "link":
				s.Offers[0].Link.To = "invented-airport"
			}
			top := routetest.Ivanovo()
			if _, err := journeys.Assemble(context.Background(), top, candidates(t, top), s, test.Query(), test.Policy(), limits); !errors.Is(err, journeys.ErrInvalidInput) {
				t.Fatal(err)
			}
		})
	}
	top := routetest.Ivanovo()
	cs := candidates(t, top)
	cs.Candidates[0].Legs[1].To = "invented-airport"
	if _, err := journeys.Assemble(context.Background(), top, cs, test.Snapshot(), test.Query(), test.Policy(), limits); !errors.Is(err, routes.ErrInvalidPlan) {
		t.Fatal("topology validation bypassed", err)
	}
	s := test.Snapshot()
	s.Offers = nil
	r := assemble(t, s, test.Policy(), test.Query())
	if len(r.Journeys) != 0 || !r.Complete() {
		t.Fatal("covered empty snapshot")
	}
}
