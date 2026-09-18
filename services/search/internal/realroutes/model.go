// Package realroutes discovers candidate routes using independent transport observations.
package realroutes

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
)

type FlightConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	MaxRequests int  `mapstructure:"max_requests"`
	MaxOptions  int  `mapstructure:"max_options"`
}

type Config struct {
	GoogleFlights   FlightConfig  `mapstructure:"google_flights"`
	CollectorBinary string        `mapstructure:"collector_binary"`
	Timeout         time.Duration `mapstructure:"timeout"`
	AirportRadiusKM float64       `mapstructure:"airport_radius_km"`
	HubRadiusKM     float64       `mapstructure:"hub_radius_km"`
	RailRadiusKM    float64       `mapstructure:"rail_radius_km"`
	MaxAirports     int           `mapstructure:"max_airports"`
	MaxHubs         int           `mapstructure:"max_hubs"`
	MaxPages        int           `mapstructure:"max_pages"`
	MaxRequests     int           `mapstructure:"max_requests"`
	MaxCandidates   int           `mapstructure:"max_candidates"`
}

func (c Config) Validate() error {
	if c.GoogleFlights.Enabled && (c.GoogleFlights.MaxRequests < 1 || c.GoogleFlights.MaxRequests > 120 || c.GoogleFlights.MaxOptions < 1 || c.GoogleFlights.MaxOptions > 50) {
		return errors.New("invalid Google Flights planner limits")
	}
	if c.HubRadiusKM <= 0 || c.HubRadiusKM > 2000 || math.IsNaN(c.HubRadiusKM) || c.CollectorBinary == "" || c.Timeout < time.Second || c.Timeout > 15*time.Minute || c.AirportRadiusKM <= 0 || c.AirportRadiusKM > 300 || math.IsNaN(c.AirportRadiusKM) || c.RailRadiusKM <= 0 || c.RailRadiusKM > 50 || math.IsNaN(c.RailRadiusKM) || c.MaxAirports < 1 || c.MaxAirports > 10 || c.MaxHubs < 1 || c.MaxHubs > 30 || c.MaxPages < 1 || c.MaxPages > 5 || c.MaxRequests < 1 || c.MaxRequests > 300 || c.MaxCandidates < 1 || c.MaxCandidates > 50 {
		return errors.New("invalid real planner limits")
	}
	return nil
}

type Query struct {
	DepartureFrom   string          `json:"departure_from,omitempty"`
	DepartureTo     string          `json:"departure_to,omitempty"`
	Adults          int             `json:"adults,omitempty"`
	OriginName      string          `json:"origin_name"`
	DestinationName string          `json:"destination_name"`
	Origin          transport.Point `json:"origin"`
	Destination     transport.Point `json:"destination"`
}
type Provider interface {
	Call(context.Context, transport.Request) (transport.Response, error)
}
type Step struct {
	ObservedDates []string `json:"observed_dates,omitempty"`
	FromCode      string   `json:"from_code,omitempty"`
	ToCode        string   `json:"to_code,omitempty"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Mode          string   `json:"mode"`
	Evidence      string   `json:"evidence"`
	Number        string   `json:"number,omitempty"`
}
type Candidate struct {
	Steps    []Step   `json:"steps"`
	Warnings []string `json:"warnings"`
}
type FlightCheck struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Date    string `json:"date"`
	Outcome string `json:"outcome"`
}
type Result struct {
	FlightChecks     []FlightCheck `json:"flight_checks,omitempty"`
	Query            Query         `json:"query"`
	ObservedAt       time.Time     `json:"observed_at"`
	Source           string        `json:"source"`
	Complete         bool          `json:"complete"`
	LimitReached     bool          `json:"limit_reached"`
	ProviderFailures int           `json:"provider_failures"`
	Requests         int           `json:"requests"`
	Issues           []string      `json:"issues"`
	Candidates       []Candidate   `json:"candidates"`
}
type Planner struct {
	Provider Provider
	Airports []airports.Airport
	Config   Config
}

func distance(a, b transport.Point) float64 {
	rad := math.Pi / 180
	lat := (b.Latitude - a.Latitude) * rad
	lon := (b.Longitude - a.Longitude) * rad
	v := math.Sin(lat/2)*math.Sin(lat/2) + math.Cos(a.Latitude*rad)*math.Cos(b.Latitude*rad)*math.Sin(lon/2)*math.Sin(lon/2)
	return 6371 * 2 * math.Asin(math.Sqrt(math.Min(1, v)))
}
func point(a airports.Airport) transport.Point {
	return transport.Point{Latitude: a.Latitude, Longitude: a.Longitude}
}
