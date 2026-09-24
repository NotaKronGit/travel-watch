package rail

import (
	"testing"
	"time"
)

func seatQuery() SeatQuery {
	return SeatQuery{From: Ref{"p", "a"}, To: Ref{"p", "b"}, TrainNumber: "016А", DepartureDate: "2027-01-10"}
}
func TestSeatQuery(t *testing.T) {
	if err := seatQuery().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*SeatQuery){
		func(q *SeatQuery) { q.To.Provider = "other" },
		func(q *SeatQuery) { q.To = q.From },
		func(q *SeatQuery) { q.From.Code = "" },
		func(q *SeatQuery) { q.TrainNumber = "" },
		func(q *SeatQuery) { q.TrainNumber = "012345678901234567890" },
		func(q *SeatQuery) { q.DepartureDate = "2027-1-10" },
		func(q *SeatQuery) { q.DepartureDate = "2027-02-30" },
	} {
		q := seatQuery()
		change(&q)
		if q.Validate() == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
}

func seatResult() SeatResult {
	departure := time.Date(2027, 1, 10, 6, 0, 0, 0, time.UTC)
	price := &Money{MinorUnits: 315000, Currency: "RUB"}
	return SeatResult{Provider: "p", ObservedAt: departure.Add(-time.Hour), Complete: true, TrainNumber: "016А", Departure: departure, Arrival: departure.Add(time.Hour), Cars: []Car{
		{Number: "01", Class: ClassPlatzkart, Seats: []Seat{{Number: "001", Position: PositionLower, Price: price}, {Number: "053", Position: PositionSideLower}}},
		{Number: "02", Class: ClassCoupe, Seats: []Seat{{Number: "001", Position: PositionUnknown, Price: price}}},
		{Number: "03", Class: ClassSV, Seats: []Seat{}},
		{Number: "04", Class: ClassSeated, Seats: []Seat{{Number: "001", Position: PositionSeat}}},
	}}
}
func TestSeatResult(t *testing.T) {
	// A car with no free seats, an unknown price, an unknown position and the same seat
	// number in different cars are all valid.
	if err := seatResult().Validate(seatQuery()); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*SeatResult){
		"other provider":      func(r *SeatResult) { r.Provider = "other" },
		"no observation time": func(r *SeatResult) { r.ObservedAt = time.Time{} },
		"arrival first":       func(r *SeatResult) { r.Arrival = r.Departure },
		"nil cars":            func(r *SeatResult) { r.Cars = nil },
		"duplicate car":       func(r *SeatResult) { r.Cars[1].Number = "01" },
		"unknown class":       func(r *SeatResult) { r.Cars[0].Class = "deluxe" },
		"nil seats":           func(r *SeatResult) { r.Cars[2].Seats = nil },
		"duplicate seat":      func(r *SeatResult) { r.Cars[0].Seats[1].Number = "001" },
		"side seat in coupe":  func(r *SeatResult) { r.Cars[1].Seats[0].Position = PositionSideLower },
		"tier in seated car":  func(r *SeatResult) { r.Cars[3].Seats[0].Position = PositionLower },
		"negative price":      func(r *SeatResult) { r.Cars[0].Seats[0].Price = &Money{MinorUnits: -1, Currency: "RUB"} },
		"bad currency":        func(r *SeatResult) { r.Cars[0].Seats[0].Price = &Money{MinorUnits: 1, Currency: "rub"} },
	} {
		r := seatResult()
		change(&r)
		if r.Validate(seatQuery()) == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
