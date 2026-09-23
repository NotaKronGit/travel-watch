package schedules

import (
	"context"
	"errors"
	"strings"
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

// An unknown transfer duration is left to the traveller: it neither blocks
// compatibility nor makes the journey preliminary.
func TestOvernightConnectionBoundaryAndUnknownTransfer(t *testing.T) {
	for _, tc := range []struct {
		name, departure, state string
		known                  bool
		count                  int
	}{
		{"exact boundary", "2027-01-11T01:40:00+03:00", "compatible", true, 1},
		{"one second short", "2027-01-11T01:39:59+03:00", "no_match", true, 0},
		{"unknown transfer", "2027-01-11T01:40:00+03:00", "compatible", false, 1},
		{"offset changes instant", "2027-01-11T01:40:00+04:00", "no_match", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := policy()
			if tc.known {
				c.Transfers = []Transfer{{From: "s2", To: "s3", Duration: time.Hour, Source: "synthetic"}}
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
				if !s.Journeys[0].TimingVerified {
					t.Fatal("unknown transfer made the journey preliminary")
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
	c.Transfers = []Transfer{{From: "s2", To: "s3", Duration: time.Hour, Source: "synthetic"}}
	p := fake("2027-01-11T03:00:00+03:00")
	if _, err := (Checker{Provider: p, Config: c}).Check(context.Background(), fixture(), ""); err == nil {
		t.Fatal("invented origin timezone")
	}
	q := fixture()
	q.Candidates[0].Steps = append([]realroutes.Step{{Mode: "transfer", From: "City", To: "A"}}, q.Candidates[0].Steps...)
	r, err := (Checker{Provider: p, Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || r.Schemes[0].State != "compatible" {
		t.Fatal("unknown initial transfer blocked the scheme", err, r)
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

func TestTransferRulesMatchCodesNotTitles(t *testing.T) {
	c := policy()
	c.Transfers = []Transfer{{From: "B", To: "C", Duration: time.Hour, Source: "synthetic"}}
	if _, err := (Checker{Provider: fake("2027-01-11T01:40:00+03:00"), Config: c}).Check(context.Background(), fixture(), "Europe/Moscow"); err == nil {
		t.Fatal("title-based transfer rule accepted")
	}
	// The origin city and its station share the title "A"; only codes tell them apart.
	q := fixture()
	q.Query.OriginID, q.Query.OriginName = "k", "A"
	q.Candidates[0].Steps = append([]realroutes.Step{{Mode: "transfer", From: "A", To: "A"}}, q.Candidates[0].Steps...)
	c.Transfers = []Transfer{{From: "s2", To: "s3", Duration: time.Hour, Source: "synthetic"}, {From: "city:k", To: "s1", Duration: 30 * time.Minute, Source: "synthetic"}}
	r, err := (Checker{Provider: fake("2027-01-11T01:40:00+03:00"), Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || r.Schemes[0].State != "compatible" {
		t.Fatal("coded initial transfer not applied", err, r)
	}
	if !contains(r.Schemes[0].Warnings, "Переезд A (город) → A (ж/д станция): 30m0s, источник оценки: synthetic") {
		t.Fatalf("warnings %q", r.Schemes[0].Warnings)
	}
	// A result saved before city ids existed cannot match a coded rule.
	q.Query.OriginID = ""
	r, err = (Checker{Provider: fake("2027-01-11T01:40:00+03:00"), Config: c}).Check(context.Background(), q, "Europe/Moscow")
	if err != nil || contains(r.Schemes[0].Warnings, "Переезд A (город) → A (ж/д станция): 30m0s, источник оценки: synthetic") {
		t.Fatal("legacy transfer treated as known", err, r)
	}
	if q.Candidates[0].Steps[0].FromPoint != nil {
		t.Fatal("check mutated the saved scheme")
	}
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func TestTransferWindowShownBetweenLegs(t *testing.T) {
	r, err := (Checker{Provider: fake("2027-01-11T03:00:00+03:00"), Config: policy()}).Check(context.Background(), fixture(), "Europe/Moscow")
	if err != nil || len(r.Schemes[0].Journeys) != 1 {
		t.Fatal(err, r)
	}
	for _, w := range r.Schemes[0].Warnings {
		if strings.Contains(w, "Неизвестно") || strings.Contains(w, "предварительные") {
			t.Fatalf("unknown transfer still reported: %q", w)
		}
	}
	legs := r.Schemes[0].Journeys[0].Legs
	if legs[0].TransferFrom != "" || legs[0].BoardingMinutes != 0 {
		t.Fatalf("first leg got a transfer: %+v", legs[0])
	}
	// Train arrives 23:00, flight leaves 03:00: 240 min, of which 60 min check-in.
	if legs[1].TransferFrom != "B" || legs[1].TransferTo != "C" || legs[1].ConnectionMinutes != 240 || legs[1].BoardingMinutes != 60 {
		t.Fatalf("transfer window %+v", legs[1])
	}
}
func TestIncompleteCheckKeepsFoundJourneys(t *testing.T) {
	// The source fails for every date except those with the synthetic departures.
	p := providerFunc(func(ctx context.Context, q transport.Request) (transport.Response, error) {
		if q.Date != "2027-01-10" && q.Date != "2027-01-11" {
			return transport.Response{}, errors.New("unavailable")
		}
		return fake("2027-01-11T03:00:00+03:00")(ctx, q)
	})
	r, err := (Checker{Provider: p, Config: policy()}).Check(context.Background(), fixture(), "Europe/Moscow")
	if err != nil || !r.Incomplete || r.Schemes[0].State != "compatible" || len(r.Schemes[0].Journeys) != 1 {
		t.Fatal("found journey lost to incompleteness", err, r)
	}
	if !contains(r.Schemes[0].Warnings, "Не удалось полностью проверить расписания: ошибка источника, неполная выдача, неподдерживаемые коды или лимит запросов.") {
		t.Fatal("incompleteness not reported", r.Schemes[0].Warnings)
	}
}
