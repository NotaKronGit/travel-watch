package realroutes

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
)

type fakeProvider struct {
	calls int
	fail  bool
}

func station(c string) transport.Station { return transport.Station{Code: c, Title: c, IATA: c} }
func connection(a, b, mode string) transport.Connection {
	return transport.Connection{From: station(a), To: station(b), Mode: mode}
}
func (f *fakeProvider) Call(ctx context.Context, q transport.Request) (transport.Response, error) {
	f.calls++
	r := transport.Response{Version: 1}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if f.fail {
		return r, errors.New("provider offline")
	}
	switch q.Method {
	case "settlement":
		r.Stations = []transport.Station{station("origin-city")}
	case "arrivals":
		r.Threads = []transport.Thread{{UID: "unknown-hub-flight", Number: "1"}}
		r.Count = 1
		r.Total = 1
	case "thread":
		r.Connections = []transport.Connection{connection("HUB", "DST", "plane")}
	case "stations":
		r.Stations = []transport.Station{station("hub-rail")}
	case "search":
		if q.Mode == "train" && q.From == "origin-city" && q.To == "hub-rail" {
			r.Connections = []transport.Connection{connection("origin-rail", "hub-rail", "train")}
		}
		if q.Mode == "plane" && q.From == "ORG" && (q.To == "HUB" || q.To == "ALT") {
			r.Connections = []transport.Connection{connection(q.From, q.To, "plane")}
		}
	}
	return r, nil
}
func fixture() (Planner, Query) {
	c := Config{CollectorBinary: "unused", Timeout: time.Minute, AirportRadiusKM: 180, HubRadiusKM: 1000, RailRadiusKM: 50, MaxAirports: 3, MaxHubs: 6, MaxPages: 2, MaxRequests: 100, MaxCandidates: 20}
	aa := []airports.Airport{}
	for i, v := range []struct {
		code string
		lon  float64
	}{{"ORG", 0}, {"HUB", 5}, {"ALT", 5.2}, {"DST", 20}} {
		aa = append(aa, airports.Airport{SourceID: int64(i + 1), IATA: v.code, Name: v.code, Longitude: v.lon, Scheduled: true, Type: "large_airport"})
	}
	return Planner{Provider: &fakeProvider{}, Airports: aa, Config: c}, Query{OriginName: "origin", DestinationName: "destination", Destination: transport.Point{Longitude: 20}}
}
func TestDiscoversHubAndDifferentAirports(t *testing.T) {
	p, q := fixture()
	r, err := p.Plan(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Candidates) != 3 {
		t.Fatalf("want train, same-airport flights, different-airport flights; got %+v", r)
	}
	found := false
	for _, c := range r.Candidates {
		if len(c.Warnings) == 0 {
			t.Fatal("missing unverified transfer warning")
		}
		for _, s := range c.Steps {
			if s.From == "ALT" && s.To == "HUB" && s.Mode == "transfer" && s.Evidence == "assumed" {
				found = true
			}
		}
	}
	if !found || r.Complete {
		t.Fatal("airport transfer missing or completeness overstated")
	}
}
func TestBoundsErrorsAndCancellation(t *testing.T) {
	p, q := fixture()
	p.Config.MaxRequests = 2
	r, err := p.Plan(context.Background(), q)
	if err != nil || !r.LimitReached || r.Requests > 2 {
		t.Fatal("request bound not enforced")
	}
	p, q = fixture()
	p.Provider = &fakeProvider{fail: true}
	r, err = p.Plan(context.Background(), q)
	if err != nil || len(r.Issues) == 0 || r.Complete || len(r.Candidates) != 0 {
		t.Fatal("outage hidden")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = p.Plan(ctx, q); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel lost")
	}
	p, q = fixture()
	p.Config.MaxCandidates = 1
	r, err = p.Plan(context.Background(), q)
	if err != nil || len(r.Candidates) != 1 || !r.LimitReached {
		t.Fatal("candidate limit lost")
	}
}

type providerFunc func(context.Context, transport.Request) (transport.Response, error)

func (f providerFunc) Call(ctx context.Context, q transport.Request) (transport.Response, error) {
	return f(ctx, q)
}

func TestGeographicDiscoveryWithoutArrivalCoverage(t *testing.T) {
	p, q := fixture()
	base := &fakeProvider{}
	p.Provider = providerFunc(func(ctx context.Context, req transport.Request) (transport.Response, error) {
		if req.Method == "arrivals" {
			return transport.Response{}, errors.New("arrivals unavailable")
		}
		if req.Method == "search" && req.From == "HUB" && req.To == "DST" {
			leg := connection("HUB", "DST", "plane")
			leg.From.IATA, leg.To.IATA = "", ""
			return transport.Response{Connections: []transport.Connection{leg}}, nil
		}
		return base.Call(ctx, req)
	})
	r, err := p.Plan(context.Background(), q)
	if err != nil || len(r.Candidates) != 3 || r.Complete {
		t.Fatalf("geographic discovery lost: %+v, %v", r, err)
	}
	for _, candidate := range r.Candidates {
		for _, step := range candidate.Steps {
			if step.Evidence == "yandex-rasp" && (step.FromCode == "" || step.ToCode == "") {
				t.Fatal("physical station identity lost")
			}
		}
	}
}

func TestAmbiguousHubIsNotJoined(t *testing.T) {
	p, q := fixture()
	p.Airports = append(p.Airports, p.Airports[1])
	r, err := p.Plan(context.Background(), q)
	if err != nil || len(r.Candidates) != 0 || len(r.Issues) == 0 {
		t.Fatalf("ambiguous airport used: %+v, %v", r, err)
	}
}

func TestEachDestinationGetsDiscoveryBudget(t *testing.T) {
	p, q := fixture()
	p.Config.MaxRequests = 30
	p.Airports = append(p.Airports, airports.Airport{SourceID: 10, IATA: "DS2", Longitude: 20.1, Scheduled: true, Type: "large_airport"})
	visited := map[string]bool{}
	p.Provider = providerFunc(func(_ context.Context, req transport.Request) (transport.Response, error) {
		r := transport.Response{}
		if req.Method == "arrivals" {
			visited[req.Code] = true
			r.Count, r.Total = 100, 1000
			for i := 0; i < 100; i++ {
				r.Threads = append(r.Threads, transport.Thread{UID: fmt.Sprintf("%s-%d", req.Code, i)})
			}
		}
		return r, nil
	})
	r, err := p.Plan(context.Background(), q)
	if err != nil || !visited["DST"] || !visited["DS2"] || r.Requests > 30 || !r.LimitReached {
		t.Fatalf("destination starved: %v, %+v, %v", visited, r, err)
	}
}
