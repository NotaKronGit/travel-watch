package schedules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
)

type providerFunc func(context.Context, transport.Request) (transport.Response, error)

func (f providerFunc) Call(ctx context.Context, q transport.Request) (transport.Response, error) {
	return f(ctx, q)
}
func policy() Config {
	return Config{Timeout: time.Minute, MaxRequests: 100, MaxDates: 2, MaxCombinations: 1000, MaxJourneys: 3, BeforeTrain: 10 * time.Minute, AfterTrain: 10 * time.Minute, BeforePlane: time.Hour, AfterPlane: 30 * time.Minute, Buffer: 30 * time.Minute, MaxConnection: 24 * time.Hour, MaxJourney: 48 * time.Hour}
}
func stamp(s string) time.Time {
	t, e := time.Parse(time.RFC3339, s)
	if e != nil {
		panic(e)
	}
	return t
}
func fixture() realroutes.Result {
	return realroutes.Result{Complete: true, Query: realroutes.Query{DepartureFrom: "2027-01-10", DepartureTo: "2027-01-10"}, Candidates: []realroutes.Candidate{{Steps: []realroutes.Step{{Mode: "train", From: "A", To: "B", FromCode: "s1", ToCode: "s2"}, {Mode: "transfer", From: "B", To: "C"}, {Mode: "plane", From: "C", To: "D", FromCode: "s3", ToCode: "s4"}}}}}
}
func fake(flight string) providerFunc {
	return func(_ context.Context, q transport.Request) (transport.Response, error) {
		r := transport.Response{Version: 1, ObservedAt: stamp("2026-09-20T10:00:00Z")}
		var d transport.Departure
		if q.From == "s1" {
			d = transport.Departure{From: transport.Station{Code: "s1", Title: "A"}, To: transport.Station{Code: "s2", Title: "B"}, Mode: "train", Number: "T1", Departure: stamp("2027-01-10T21:00:00+03:00"), Arrival: stamp("2027-01-10T23:00:00+03:00")}
		} else {
			d = transport.Departure{From: transport.Station{Code: "s3", Title: "C"}, To: transport.Station{Code: "s4", Title: "D"}, Mode: "plane", Number: "F1", Departure: stamp(flight), Arrival: stamp(flight).Add(3 * time.Hour)}
		}
		if d.Departure.Format(time.DateOnly) == q.Date {
			r.Departures = []transport.Departure{d}
			r.Count = 1
			r.Total = 1
		}
		return r, nil
	}
}
func TestOvernightConnectionBoundaryAndUnknownTransfer(t *testing.T) {
	for _, tc := range []struct {
		name, departure, state string
		known                  bool
		count                  int
	}{
		{"exact boundary", "2027-01-11T01:40:00+03:00", "compatible", true, 1},
		{"one second short", "2027-01-11T01:39:59+03:00", "no_match", true, 0},
		{"unknown transfer", "2027-01-11T01:40:00+03:00", "unverified", false, 1},
		{"offset changes instant", "2027-01-11T01:40:00+04:00", "no_match", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := policy()
			if tc.known {
				c.Transfers = []Transfer{{From: "B", To: "C", Duration: time.Hour, Source: "synthetic"}}
			}
			r, err := (Checker{Provider: fake(tc.departure), Config: c}).Check(context.Background(), fixture(), "Europe/Moscow")
			if err != nil {
				t.Fatal(err)
			}
			s := r.Schemes[0]
			if s.State != tc.state || len(s.Journeys) != tc.count {
				t.Fatalf("unexpected result %+v", s)
			}
			if tc.count > 0 {
				if s.Journeys[0].TimingVerified != tc.known {
					t.Fatal("unknown transfer verified")
				}
				if s.Journeys[0].Legs[0].ObservedAt.AsTime() != stamp("2026-09-20T10:00:00Z") {
					t.Fatal("observation changed")
				}
			}
		})
	}
}
func TestProviderErrorBudgetAndReuse(t *testing.T) {
	c := policy()
	q := fixture()
	q.Candidates = append(q.Candidates, q.Candidates[0])
	calls := map[transport.Request]int{}
	p := providerFunc(func(ctx context.Context, q transport.Request) (transport.Response, error) {
		calls[q]++
		return fake("2027-01-11T03:00:00+03:00")(ctx, q)
	})
	r, err := (Checker{Provider: p, Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || len(r.Schemes) != 2 {
		t.Fatal(err)
	}
	for _, n := range calls {
		if n != 1 {
			t.Fatal("duplicate provider request")
		}
	}
	c.MaxRequests = 1
	r, err = (Checker{Provider: p, Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || r.Requests != 1 || !r.Incomplete {
		t.Fatal("budget not enforced", err)
	}
	p = func(context.Context, transport.Request) (transport.Response, error) {
		return transport.Response{}, errors.New("unavailable")
	}
	r, err = (Checker{Provider: p, Config: policy()}).Check(context.Background(), fixture(), "Europe/Moscow")
	if err != nil || r.Schemes[0].State != "unverified" || !r.Incomplete {
		t.Fatal("error became empty schedule", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (Checker{Provider: p, Config: policy()}).Check(ctx, fixture(), "Europe/Moscow")
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
}
func TestUnknownTimezoneAndInitialTransfer(t *testing.T) {
	c := policy()
	c.Transfers = []Transfer{{From: "B", To: "C", Duration: time.Hour, Source: "synthetic"}}
	p := fake("2027-01-11T03:00:00+03:00")
	if _, err := (Checker{Provider: p, Config: c}).Check(context.Background(), fixture(), ""); err == nil {
		t.Fatal("invented origin timezone")
	}
	q := fixture()
	q.Candidates[0].Steps = append([]realroutes.Step{{Mode: "transfer", From: "City", To: "A"}}, q.Candidates[0].Steps...)
	r, err := (Checker{Provider: p, Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || r.Schemes[0].State != "unverified" {
		t.Fatal("unknown initial transfer accepted", err)
	}
}

func TestDepartureWindowAndPhysicalAirportIdentity(t *testing.T) {
	c := policy()
	q := fixture()
	q.Candidates[0].Steps = []realroutes.Step{{Mode: "plane", From: "A", To: "B", FromCode: "s1", ToCode: "s2"}}
	p := providerFunc(func(_ context.Context, request transport.Request) (transport.Response, error) {
		r := transport.Response{Version: 1, ObservedAt: stamp("2026-09-20T10:00:00Z")}
		if request.Date == "2027-01-10" {
			r.Count = 1
			r.Total = 1
			r.Departures = []transport.Departure{{From: transport.Station{Code: "s1", Title: "A"}, To: transport.Station{Code: "s2", Title: "B"}, Mode: "plane", Number: "F1", Departure: stamp("2027-01-10T00:30:00+03:00"), Arrival: stamp("2027-01-10T02:00:00+03:00")}}
		}
		return r, nil
	})
	r, err := (Checker{Provider: p, Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || len(r.Schemes[0].Journeys) != 0 || r.Schemes[0].State != "no_match" {
		t.Fatal("departure requires boarding on previous date", err)
	}
	c.BeforePlane = 0
	r, err = (Checker{Provider: p, Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || len(r.Schemes[0].Journeys) != 1 {
		t.Fatal("date boundary rejected", err)
	}
	q.Candidates[0].Steps[0].ToCode = "s99"
	r, err = (Checker{Provider: p, Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || r.Schemes[0].State != "unverified" || len(r.Schemes[0].Journeys) != 0 {
		t.Fatal("wrong airport accepted", err)
	}
}
