package main

import (
	"context"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/flights"
)

type flightStub struct{ q flights.Query }

func (p *flightStub) FindFlights(_ context.Context, q flights.Query) (flights.Result, error) {
	p.q = q
	return flights.Result{ObservedAt: time.Now(), Query: q, Options: []flights.Option{{DurationMinutes: 600, Legs: []flights.Leg{{From: "DME", To: "CAI", Number: "TEST1", DepartureLocal: "2027-01-01T01:00:00"}, {From: "CAI", To: "SSH", Number: "TEST2"}}}}}, nil
}
func TestFlightProtocolOnlyTopology(t *testing.T) {
	p := &flightStub{}
	r, err := flightPaths(context.Background(), p, transport.Request{Version: 1, From: "DME", To: "SSH", Date: "2027-01-01", Adults: 2, Limit: 5})
	if err != nil || len(r.FlightPaths) != 1 || len(r.FlightPaths[0].Legs) != 2 || r.FlightPaths[0].Legs[0].To.IATA != "CAI" || r.FlightPaths[0].Legs[0].Number != "" || p.q.Nonstop || p.q.Adults != 2 {
		t.Fatal(r, err)
	}
}
