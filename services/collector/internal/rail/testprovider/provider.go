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
// SeatsIncomplete drops the last car and marks the seat result incomplete.
// Configure before use; don't mutate the fields concurrently with requests.
type Provider struct {
	StationError, TrainError, SeatError error
	SeatsIncomplete                     bool
}

var _ rail.StationProvider = Provider{}
var _ rail.TrainProvider = Provider{}
var _ rail.SeatProvider = Provider{}

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
	if q.Center != nil || q.Country != "RU" {
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

// FindSeats knows train TEST from test-origin to test-destination on 2027-01-10, the
// first offer of FindTrains. Other TEST dates are not on sale; other trains are unknown.
func (p Provider) FindSeats(ctx context.Context, q rail.SeatQuery) (rail.SeatResult, error) {
	if err := ctx.Err(); err != nil {
		return rail.SeatResult{}, err
	}
	if err := q.Validate(); err != nil {
		return rail.SeatResult{}, err
	}
	if p.SeatError != nil {
		return rail.SeatResult{}, p.SeatError
	}
	if q.From != ref("test-origin") || q.To != ref("test-destination") {
		return rail.SeatResult{}, rail.ErrUnsupported
	}
	if q.TrainNumber != "TEST" {
		return rail.SeatResult{}, rail.ErrTrainNotFound
	}
	if q.DepartureDate != "2027-01-10" {
		return rail.SeatResult{}, rail.ErrNotOnSale
	}
	zone := time.FixedZone("test-UTC+3", 3*60*60)
	departure := time.Date(2027, 1, 10, 6, 0, 0, 0, zone)
	r := rail.SeatResult{Provider: ID, Synthetic: true, ObservedAt: observed(), Complete: true, TrainNumber: "TEST", Departure: departure, Arrival: departure.Add(4 * time.Hour), Cars: cars()}
	if p.SeatsIncomplete {
		r.Cars = r.Cars[:len(r.Cars)-1]
		r.Complete = false
	}
	if err := r.Validate(q); err != nil {
		return rail.SeatResult{}, err
	}
	return r, nil
}

func rub(rubles int64) *rail.Money { return &rail.Money{MinorUnits: rubles * 100, Currency: "RUB"} }
func seat(number string, position rail.SeatPosition, rubles int64) rail.Seat {
	return rail.Seat{Number: number, Position: position, Price: rub(rubles)}
}

// cars builds a fresh fixture on every call so callers cannot share mutable data.
// Platzkart and coupe positions follow standard numbering: odd lower, even upper; 37–54 side.
func cars() []rail.Car {
	return []rail.Car{
		{Number: "01", Class: rail.ClassPlatzkart, ServiceClass: "3Э", Seats: []rail.Seat{
			seat("001", rail.PositionLower, 3150), seat("002", rail.PositionUpper, 2900),
			seat("013", rail.PositionLower, 3150), seat("014", rail.PositionUpper, 2900),
			seat("053", rail.PositionSideLower, 2700), seat("054", rail.PositionSideUpper, 2500),
		}},
		{Number: "02", Class: rail.ClassCoupe, ServiceClass: "2К", Seats: []rail.Seat{
			seat("005", rail.PositionLower, 6900), seat("006", rail.PositionUpper, 6500),
			seat("007", rail.PositionLower, 6900), seat("008", rail.PositionUpper, 6500),
			seat("012", rail.PositionUpper, 6500),
		}},
		{Number: "03", Class: rail.ClassSV, ServiceClass: "1Б", Seats: []rail.Seat{}},
		{Number: "04", Class: rail.ClassSeated, ServiceClass: "2С", Seats: []rail.Seat{
			seat("001", rail.PositionSeat, 1800),
			{Number: "002", Position: rail.PositionSeat},
		}},
	}
}
