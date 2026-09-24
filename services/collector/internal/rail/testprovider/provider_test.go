package testprovider

import (
	"context"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail"
	"testing"
	"time"
)

func query() rail.TrainQuery {
	return rail.TrainQuery{From: ref("test-origin"), To: ref("test-destination"), DepartureDate: "2027-01-10", Adults: 2, Limit: 10}
}
func TestStationSearch(t *testing.T) {
	p := Provider{}
	for _, tc := range []struct {
		text         string
		limit, count int
		complete     bool
	}{{"примерск", 10, 2, true}, {"Примерск", 1, 1, false}, {"Примерск", 2, 2, true}, {"not-found", 10, 0, true}} {
		r, err := p.FindStations(context.Background(), rail.StationQuery{Text: tc.text, Country: "RU", Limit: tc.limit})
		if err != nil || len(r.Stations) != tc.count || r.Complete != tc.complete || r.LimitReached == tc.complete || !r.Synthetic || r.ObservedAt.IsZero() {
			t.Fatalf("unexpected result %+v %v", r, err)
		}
		for _, s := range r.Stations {
			if s.Ref.Provider != ID || s.Coordinates != nil {
				t.Fatal("identity or unknown coordinates lost")
			}
		}
	}
	_, err := p.FindStations(context.Background(), rail.StationQuery{Text: "city", Country: "JP", Limit: 10})
	if !errors.Is(err, rail.ErrUnsupported) {
		t.Fatal("unsupported became empty", err)
	}
}
func TestOffersAndIsolation(t *testing.T) {
	p := Provider{}
	q := query()
	r, err := p.FindTrains(context.Background(), q)
	if err != nil || !r.Complete || !r.Synthetic || len(r.Offers) != 2 {
		t.Fatal(r, err)
	}
	for _, o := range r.Offers {
		if o.From != q.From || o.To != q.To || o.Adults != 2 || o.Price.MinorUnits != 200000 || !o.Arrival.After(o.Departure) || o.Departure.Format(time.DateOnly) != q.DepartureDate || o.ObservedAt.IsZero() {
			t.Fatal(o)
		}
	}
	r.Offers[0].Price.MinorUnits = 0
	r.Offers[0].From.Code = "changed"
	fresh, err := p.FindTrains(context.Background(), q)
	if err != nil || fresh.Offers[0].Price.MinorUnits != 200000 || fresh.Offers[0].From != q.From {
		t.Fatal("mutable fixture shared")
	}
	q.Limit = 1
	r, err = p.FindTrains(context.Background(), q)
	if err != nil || r.Complete || !r.LimitReached || len(r.Offers) != 1 {
		t.Fatal("limit lost")
	}
	q = query()
	q.To = ref("test-other")
	if _, err = p.FindTrains(context.Background(), q); !errors.Is(err, rail.ErrUnsupported) {
		t.Fatal(err)
	}
}
func TestFailureAndCancellation(t *testing.T) {
	stationQuery := rail.StationQuery{Text: "Тест", Country: "RU", Limit: 10}
	for _, failure := range []error{rail.ErrUnavailable, rail.ErrRateLimited} {
		p := Provider{StationError: failure, TrainError: failure}
		sr, err := p.FindStations(context.Background(), stationQuery)
		if !errors.Is(err, failure) || sr.Complete || len(sr.Stations) != 0 {
			t.Fatal("station failure lost")
		}
		tr, err := p.FindTrains(context.Background(), query())
		if !errors.Is(err, failure) || tr.Complete || len(tr.Offers) != 0 {
			t.Fatal("train failure lost")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Provider{}).FindStations(ctx, stationQuery); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := (Provider{}).FindTrains(ctx, query()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := (Provider{}).FindTrains(ctx, query()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestInvalidQueries(t *testing.T) {
	for _, change := range []func(*rail.TrainQuery){func(q *rail.TrainQuery) { q.To.Provider = "another-provider" }, func(q *rail.TrainQuery) { q.To = q.From }, func(q *rail.TrainQuery) { q.DepartureDate = "2027-02-30" }, func(q *rail.TrainQuery) { q.Adults = 0 }, func(q *rail.TrainQuery) { q.Limit = 101 }} {
		q := query()
		change(&q)
		if _, err := (Provider{}).FindTrains(context.Background(), q); !errors.Is(err, rail.ErrInvalidQuery) {
			t.Fatal("invalid query accepted")
		}
	}
	if _, err := (Provider{}).FindStations(context.Background(), rail.StationQuery{Country: "RU", Limit: 10}); !errors.Is(err, rail.ErrInvalidQuery) {
		t.Fatal(err)
	}
}
func seatQuery() rail.SeatQuery {
	return rail.SeatQuery{From: ref("test-origin"), To: ref("test-destination"), TrainNumber: "TEST", DepartureDate: "2027-01-10"}
}
func TestSeats(t *testing.T) {
	p := Provider{}
	r, err := p.FindSeats(context.Background(), seatQuery())
	if err != nil || !r.Complete || !r.Synthetic || r.ObservedAt.IsZero() || len(r.Cars) != 4 {
		t.Fatal(r, err)
	}
	// The run matches the first FindTrains offer.
	offers, err := p.FindTrains(context.Background(), query())
	if err != nil || !r.Departure.Equal(offers.Offers[0].Departure) || !r.Arrival.Equal(offers.Offers[0].Arrival) {
		t.Fatal("seat run differs from the train offer")
	}
	platzkart, coupe, sv, seated := r.Cars[0], r.Cars[1], r.Cars[2], r.Cars[3]
	if platzkart.Class != rail.ClassPlatzkart || platzkart.Seats[4].Position != rail.PositionSideLower || platzkart.Seats[5].Position != rail.PositionSideUpper {
		t.Fatal("platzkart side seats lost", platzkart)
	}
	if coupe.Class != rail.ClassCoupe || len(coupe.Seats) != 5 {
		t.Fatal("coupe lost", coupe)
	}
	if sv.Class != rail.ClassSV || sv.Seats == nil || len(sv.Seats) != 0 {
		t.Fatal("car without free seats lost", sv)
	}
	if seated.Seats[0].Position != rail.PositionSeat || seated.Seats[0].Price == nil || seated.Seats[1].Price != nil {
		t.Fatal("seated car or unknown price lost", seated)
	}
	r.Cars[0].Seats[0].Price.MinorUnits = 0
	r.Cars[0].Seats = r.Cars[0].Seats[:1]
	fresh, err := p.FindSeats(context.Background(), seatQuery())
	if err != nil || fresh.Cars[0].Seats[0].Price.MinorUnits != 315000 || len(fresh.Cars[0].Seats) != 6 {
		t.Fatal("mutable fixture shared")
	}
	partial, err := Provider{SeatsIncomplete: true}.FindSeats(context.Background(), seatQuery())
	if err != nil || partial.Complete || len(partial.Cars) != 3 {
		t.Fatal("partial result looks complete", partial, err)
	}
}
func TestSeatFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(*rail.SeatQuery)
		want   error
	}{
		"other stations": {func(q *rail.SeatQuery) { q.To = ref("test-other") }, rail.ErrUnsupported},
		"unknown train":  {func(q *rail.SeatQuery) { q.TrainNumber = "OTHER" }, rail.ErrTrainNotFound},
		"not on sale":    {func(q *rail.SeatQuery) { q.DepartureDate = "2027-06-01" }, rail.ErrNotOnSale},
		"invalid query":  {func(q *rail.SeatQuery) { q.TrainNumber = "" }, rail.ErrInvalidQuery},
	} {
		q := seatQuery()
		tc.change(&q)
		r, err := (Provider{}).FindSeats(context.Background(), q)
		if !errors.Is(err, tc.want) || r.Complete || r.Cars != nil {
			t.Fatalf("%s: %v %+v", name, err, r)
		}
	}
	for _, failure := range []error{rail.ErrUnavailable, rail.ErrRateLimited} {
		r, err := Provider{SeatError: failure}.FindSeats(context.Background(), seatQuery())
		if !errors.Is(err, failure) || r.Complete || r.Cars != nil {
			t.Fatal("seat failure lost")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Provider{}).FindSeats(ctx, seatQuery()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
