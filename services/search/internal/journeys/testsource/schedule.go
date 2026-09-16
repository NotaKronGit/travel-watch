// Package testsource supplies fictitious schedules, never live travel offers.
package testsource

import (
	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	routetest "github.com/NotaKronGit/travel-watch/services/search/internal/routes/testsource"
	"time"
)

func Policy() journeys.Policy {
	return journeys.Policy{TrainBoarding: 10 * time.Minute, FlightBoarding: time.Hour, TrainExit: 10 * time.Minute, FlightExit: 30 * time.Minute, DelayReserve: 30 * time.Minute, MaxConnection: 18 * time.Hour, MaxJourney: 48 * time.Hour}
}
func Query() journeys.Query {
	return journeys.Query{Route: routes.Query{OriginCityID: routetest.Origin, DestinationCityID: routetest.Destination}, DepartureFrom: "2027-01-10", DepartureTo: "2027-01-10", Adults: 1}
}
func Snapshot() journeys.Snapshot {
	observed := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	stamp := func(day, hour, minute, offset int) time.Time {
		return time.Date(2027, 1, day, hour, minute, 0, 0, time.FixedZone("", offset*3600))
	}
	offer := func(id, from, to string, mode routes.Mode, departure, arrival time.Time, price int64) journeys.Offer {
		return journeys.Offer{ID: id, Source: "synthetic-schedule-v1", Fare: "test-economy", Link: routes.Link{From: from, To: to, Mode: mode}, Departure: departure, Arrival: arrival, ObservedAt: observed, Adults: 1, Price: &journeys.Money{MinorUnits: price, Currency: "RUB"}}
	}
	s := journeys.Snapshot{Source: "synthetic-schedule-v1", Synthetic: true, Complete: true, Offers: []journeys.Offer{
		offer("local-direct", "test-iwa", "test-arrival", routes.Flight, stamp(10, 12, 0, 3), stamp(10, 23, 0, 7), 3000000),
		offer("train-moscow", "test-ivanovo-station", "test-moscow-station", routes.Train, stamp(10, 6, 0, 3), stamp(10, 10, 0, 3), 200000),
		offer("train-nizhny", "test-ivanovo-station", "test-nizhny-station", routes.Train, stamp(10, 6, 0, 3), stamp(10, 10, 30, 3), 150000),
		offer("moscow-flight", "test-svo", "test-arrival", routes.Flight, stamp(10, 14, 0, 3), stamp(11, 1, 0, 7), 2500000),
		offer("nizhny-too-early", "test-goj", "test-arrival", routes.Flight, stamp(10, 12, 0, 3), stamp(10, 23, 0, 7), 2300000),
		offer("nizhny-next-day", "test-goj", "test-arrival", routes.Flight, stamp(11, 0, 30, 3), stamp(11, 11, 30, 7), 2400000),
		offer("feeder-moscow", "test-iwa", "test-svo", routes.Flight, stamp(10, 9, 0, 3), stamp(10, 10, 0, 3), 500000),
		offer("feeder-nizhny", "test-iwa", "test-goj", routes.Flight, stamp(10, 9, 0, 3), stamp(10, 10, 30, 3), 400000),
	}}
	for _, link := range routetest.Ivanovo().Links {
		if link.Mode != routes.Transfer {
			continue
		}
		duration := 30 * time.Minute
		switch link.From {
		case "test-moscow-station":
			duration = 90 * time.Minute
		case "test-nizhny-station", "test-arrival":
			duration = time.Hour
		}
		s.Transfers = append(s.Transfers, journeys.TransferTiming{Link: link, Source: s.Source, ObservedAt: observed, Known: true, Duration: duration})
	}
	return s
}
