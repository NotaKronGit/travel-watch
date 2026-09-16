// Package rail defines provider-neutral railway lookup contracts inside Collector.
// It is not a wire contract between services.
package rail

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidQuery = errors.New("invalid railway query")
	ErrUnavailable  = errors.New("railway provider unavailable")
	ErrRateLimited  = errors.New("railway provider rate limited")
	ErrUnsupported  = errors.New("railway query outside provider coverage")
)

// Ref identifies an object inside one provider. Codes are not global identities.
type Ref struct{ Provider, Code string }
type Coordinates struct{ Latitude, Longitude float64 }
type Station struct {
	Ref                           Ref
	Name, Country, City, Timezone string
	Coordinates                   *Coordinates // nil means unknown; (0,0) is a coordinate.
	ESR, Express3                 string       // Optional identifiers; preserve leading zeroes.
}

// StationQuery searches by provider-interpreted text within a country.
// A result is a candidate match, not an approved city-to-station association.
type StationQuery struct {
	Text, Country string
	Limit         int
}
type TrainQuery struct {
	From, To      Ref
	DepartureDate string // YYYY-MM-DD, local calendar date at the origin station.
	Adults        int
	Limit         int
}

// Money is the price for the whole quoted adult group in currency minor units.
type Money struct {
	MinorUnits int64
	Currency   string
}
type Offer struct {
	Ref                Ref
	From, To           Ref
	TrainNumber, Fare  string
	Departure, Arrival time.Time
	Adults             int
	Price              *Money // nil means unknown, not free.
	ObservedAt         time.Time
}

// Complete applies only to this provider and query, never to all trains worldwide.
// LimitReached forces Complete=false. An error makes the result unusable.
type StationResult struct {
	Provider               string
	Synthetic              bool
	ObservedAt             time.Time
	Complete, LimitReached bool
	Stations               []Station
}
type TrainResult struct {
	Provider               string
	Synthetic              bool
	ObservedAt             time.Time
	Complete, LimitReached bool
	Offers                 []Offer
}
type StationProvider interface {
	FindStations(context.Context, StationQuery) (StationResult, error)
}
type TrainProvider interface {
	FindTrains(context.Context, TrainQuery) (TrainResult, error)
}
