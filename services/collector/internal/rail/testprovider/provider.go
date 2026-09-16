// Package testprovider supplies fixed, explicitly synthetic railway data.
package testprovider

import (
	"context"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail"
	"strings"
	"time"
)

const ID = "synthetic-rail"

// Provider can simulate transport failures without external requests or sleeping.
// Configure before use; don't mutate the fields concurrently with requests.
type Provider struct{ StationError, TrainError error }

var _ rail.StationProvider = Provider{}
var _ rail.TrainProvider = Provider{}

func ref(code string) rail.Ref { return rail.Ref{Provider: ID, Code: code} }
func observed() time.Time      { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }
func stations() []rail.Station {
	return []rail.Station{
		{Ref: ref("test-origin"), Name: "Тестовый вокзал отправления", City: "Тестоград", Country: "RU", Timezone: "Europe/Moscow"},
		{Ref: ref("test-destination"), Name: "Тестовый центральный вокзал", City: "Примерск", Country: "RU", Timezone: "Europe/Moscow"},
		{Ref: ref("test-other"), Name: "Тестовый северный вокзал", City: "Примерск", Country: "RU", Timezone: "Europe/Moscow"},
	}
}
func (p Provider) FindStations(ctx context.Context, q rail.StationQuery) (rail.StationResult, error) {
	if err := ctx.Err(); err != nil {
		return rail.StationResult{}, err
	}
	if err := q.Validate(); err != nil {
		return rail.StationResult{}, err
	}
	if p.StationError != nil {
		return rail.StationResult{}, p.StationError
	}
	if q.Country != "RU" {
		return rail.StationResult{}, rail.ErrUnsupported
	}
	r := rail.StationResult{Provider: ID, Synthetic: true, ObservedAt: observed(), Complete: true, Stations: []rail.Station{}}
	text := strings.ToLower(strings.TrimSpace(q.Text))
	for _, s := range stations() {
		if err := ctx.Err(); err != nil {
			return rail.StationResult{}, err
		}
		if !strings.Contains(strings.ToLower(s.Name+" "+s.City), text) {
			continue
		}
		if len(r.Stations) == q.Limit {
			r.Complete = false
			r.LimitReached = true
			break
		}
		r.Stations = append(r.Stations, s)
	}
	return r, nil
}
func (p Provider) FindTrains(ctx context.Context, q rail.TrainQuery) (rail.TrainResult, error) {
	if err := ctx.Err(); err != nil {
		return rail.TrainResult{}, err
	}
	if err := q.Validate(); err != nil {
		return rail.TrainResult{}, err
	}
	if p.TrainError != nil {
		return rail.TrainResult{}, p.TrainError
	}
	if q.From != ref("test-origin") || q.To != ref("test-destination") || q.DepartureDate != "2027-01-10" {
		return rail.TrainResult{}, rail.ErrUnsupported
	}
	r := rail.TrainResult{Provider: ID, Synthetic: true, ObservedAt: observed(), Complete: true, Offers: []rail.Offer{}}
	// Fixed UTC+3 times; no external timezone database is needed by this fixture.
	zone := time.FixedZone("test-UTC+3", 3*60*60)
	for i := range 2 {
		if err := ctx.Err(); err != nil {
			return rail.TrainResult{}, err
		}
		if len(r.Offers) == q.Limit {
			r.Complete = false
			r.LimitReached = true
			break
		}
		departure := time.Date(2027, 1, 10, 6+i, 0, 0, 0, zone)
		r.Offers = append(r.Offers, rail.Offer{Ref: ref([]string{"test-offer-1", "test-offer-2"}[i]), From: q.From, To: q.To, TrainNumber: "TEST", Fare: "Synthetic adult fare", Departure: departure, Arrival: departure.Add(4 * time.Hour), Adults: q.Adults, Price: &rail.Money{MinorUnits: int64(100000 * q.Adults), Currency: "RUB"}, ObservedAt: observed()})
	}
	return r, nil
}
