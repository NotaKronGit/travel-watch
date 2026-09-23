package realroutes

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
)

func TestSampleDates(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		want     []string
	}{
		{"2027-01-01", "2027-01-01", []string{"2027-01-01"}},
		{"2027-01-01", "2027-01-02", []string{"2027-01-01", "2027-01-02"}},
		{"2027-01-01", "2027-01-07", []string{"2027-01-01", "2027-01-04", "2027-01-07"}},
	} {
		got, err := sampleDates(Query{DepartureFrom: tc.from, DepartureTo: tc.to, Adults: 2})
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(got, err)
		}
	}
	if _, err := sampleDates(Query{}); err == nil {
		t.Fatal("missing dates accepted")
	}
	if _, err := sampleDates(Query{DepartureFrom: "2027-01-02", DepartureTo: "2027-01-01", Adults: 2}); err == nil {
		t.Fatal("reversed dates accepted")
	}
}

type flightFixture struct {
	base     fakeProvider
	requests []transport.Request
	fail     bool
}

func (f *flightFixture) Call(ctx context.Context, q transport.Request) (transport.Response, error) {
	if q.Method != "flight-paths" {
		return f.base.Call(ctx, q)
	}
	f.requests = append(f.requests, q)
	if f.fail {
		return transport.Response{}, errors.New("flight source down")
	}
	paths := []transport.FlightPath{}
	if q.From == "ORG" && q.To == "DST" {
		// Duplicate itineraries stand for different flight numbers with the same topology.
		path := transport.FlightPath{Legs: []transport.Connection{connection("ORG", "MID", "plane"), connection("MID", "DST", "plane")}}
		paths = append(paths, path, path)
	}
	return transport.Response{Version: 1, Incomplete: true, FlightPaths: paths}, nil
}
func TestGooglePathsIndependentDatesAndTopology(t *testing.T) {
	p, q := fixture()
	f := &flightFixture{}
	p.Provider = f
	p.Config.GoogleFlights = FlightConfig{Enabled: true, MaxRequests: 30, MaxOptions: 20}
	q.DepartureFrom, q.DepartureTo, q.Adults = "2027-01-01", "2027-01-07", 2
	r, err := p.Plan(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, candidate := range r.Candidates {
		for _, step := range candidate.Steps {
			if step.Evidence == "google-flights-fli" && step.FromCode == "iata:ORG" {
				found++
				if !reflect.DeepEqual(step.ObservedDates, []string{"2027-01-01", "2027-01-04", "2027-01-07"}) {
					t.Fatal(step)
				}
				if step.ToCode != "iata:MID" {
					t.Fatal("provider connection lost")
				}
			}
		}
	}
	if found != 1 || len(r.FlightChecks) == 0 || r.Complete {
		t.Fatal("topology not deduplicated or observations missing", found)
	}
	for _, req := range f.requests {
		if req.Adults != 2 || req.Date == "" {
			t.Fatal("query snapshot lost")
		}
	}
}
func TestGoogleFailurePreservesYandexAndBounds(t *testing.T) {
	p, q := fixture()
	p.Provider = &flightFixture{fail: true}
	p.Config.GoogleFlights = FlightConfig{Enabled: true, MaxRequests: 1, MaxOptions: 20}
	q.DepartureFrom, q.DepartureTo, q.Adults = "2027-01-01", "2027-01-07", 2
	r, err := p.Plan(context.Background(), q)
	if err != nil || len(r.Candidates) != 3 || len(r.FlightChecks) != 1 || r.ProviderFailures != 1 || !r.LimitReached {
		t.Fatal(r, err)
	}
}
func TestGoogleFlightsSkipsPastDates(t *testing.T) {
	dates := []string{"2027-01-01", "2027-01-04", "2027-01-07"}
	// 22:30 UTC on Jan 3 is already Jan 4 in Moscow.
	now := time.Date(2027, 1, 3, 22, 30, 0, 0, time.UTC)
	if got := futureDates(dates, Query{OriginTimezone: "Europe/Moscow"}, now); !reflect.DeepEqual(got, []string{"2027-01-04", "2027-01-07"}) {
		t.Fatal(got)
	}
	if got := futureDates(dates, Query{}, now); !reflect.DeepEqual(got, []string{"2027-01-04", "2027-01-07"}) {
		t.Fatal("unknown timezone should fall back to UTC", got)
	}
	p, q := fixture()
	f := &flightFixture{}
	p.Provider = f
	p.Config.GoogleFlights = FlightConfig{Enabled: true, MaxRequests: 30, MaxOptions: 20}
	q.DepartureFrom, q.DepartureTo, q.Adults, q.OriginTimezone = "2027-01-01", "2027-01-07", 2, "Europe/Moscow"
	p.Now = func() time.Time { return time.Date(2027, 1, 8, 12, 0, 0, 0, time.UTC) }
	r, err := p.Plan(context.Background(), q)
	if err != nil || len(f.requests) != 0 || len(r.FlightChecks) != 0 {
		t.Fatal("past dates requested", err, f.requests)
	}
	found := false
	for _, issue := range r.Issues {
		found = found || issue == "Google Flights skipped: all departure dates are in the past"
	}
	if !found {
		t.Fatal("skip not reported", r.Issues)
	}
}
