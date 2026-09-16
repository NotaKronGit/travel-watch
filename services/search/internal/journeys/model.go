// Package journeys assembles scheduled offers into feasible, unprotected trips.
package journeys

import (
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	"time"
)

// Money is the total quoted price for Offer.Adults, in currency minor units.
// A nil price is unknown, not zero; different currencies are not summed.
type Money struct {
	MinorUnits int64
	Currency   string
}
type Offer struct {
	ID, Source, Fare               string
	Link                           routes.Link
	Departure, Arrival, ObservedAt time.Time
	Adults                         int
	Price                          *Money
}

// TransferTiming is a constant estimate for this snapshot, not a booked transfer.
// A missing entry or Known=false cannot establish temporal feasibility.
type TransferTiming struct {
	Link       routes.Link
	Source     string
	ObservedAt time.Time
	Known      bool
	Duration   time.Duration
}
type Snapshot struct {
	Source              string
	Synthetic, Complete bool
	Offers              []Offer
	Transfers           []TransferTiming
}
type Query struct {
	Route                      routes.Query
	DepartureFrom, DepartureTo string // Inclusive dates of the full trip's start.
	Adults                     int
}
type Policy struct {
	TrainBoarding, FlightBoarding time.Duration
	TrainExit, FlightExit         time.Duration
	DelayReserve                  time.Duration
	MaxConnection, MaxJourney     time.Duration
}
type Limits struct {
	Routes                                                routes.Limits
	MaxOffers, MaxTransfers, MaxCombinations, MaxJourneys int
}
type Leg struct {
	Link       routes.Link
	Start, End time.Time
	Offer      *Offer // Nil for an estimated ground transfer.
}
type Journey struct {
	Legs       []Leg
	Start, End time.Time
}
type Result struct {
	Source                                       string
	Synthetic                                    bool
	Journeys                                     []Journey
	InputIncomplete, MissingTiming, LimitReached bool
}

func (r Result) Complete() bool { return !r.InputIncomplete && !r.MissingTiming && !r.LimitReached }
