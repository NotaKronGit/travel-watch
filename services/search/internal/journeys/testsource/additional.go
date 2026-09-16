package testsource

import (
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
)

// Scenario contains independent fictitious geography, links and quotes.
// City names illustrate the test; no route or airport service is asserted as real.
type Scenario struct {
	Name     string
	Topology routes.Snapshot
	Schedule journeys.Snapshot
	Query    journeys.Query
	Policy   journeys.Policy
}

// AdditionalRoutes returns fresh data so tests and later planner comparisons can reuse it.
func AdditionalRoutes() []Scenario {
	return []Scenario{kazanSochi(), tverDubai(), omskIstanbul(), tokyoHonolulu()}
}
func scenario(name, origin, originZone, destination, destinationZone, date string, adults int) Scenario {
	source := "synthetic-" + name
	return Scenario{Name: name, Topology: routes.Snapshot{Source: source, Synthetic: true, Complete: true, Cities: []routes.City{
		{ID: source + "-origin", Name: origin, Timezone: originZone},
		{ID: source + "-destination", Name: destination, Timezone: destinationZone},
	}}, Schedule: journeys.Snapshot{Source: source, Synthetic: true, Complete: true}, Query: journeys.Query{Route: routes.Query{OriginCityID: source + "-origin", DestinationCityID: source + "-destination"}, DepartureFrom: date, DepartureTo: date, Adults: adults}, Policy: Policy()}
}
func (s *Scenario) hub(name, zone string) string {
	id := s.Topology.Source + "-hub"
	s.Topology.Cities = append(s.Topology.Cities, routes.City{ID: id, Name: name, Timezone: zone})
	return id
}
func (s *Scenario) node(id, city string, kind routes.NodeKind) string {
	key := s.Topology.Source + "-" + id
	zone := ""
	for _, c := range s.Topology.Cities {
		if c.ID == city {
			zone = c.Timezone
		}
	}
	s.Topology.Nodes = append(s.Topology.Nodes, routes.Node{ID: key, CityID: city, Name: "Test " + id, Timezone: zone, Kind: kind})
	return key
}
func (s *Scenario) link(from, to string, mode routes.Mode) routes.Link {
	link := routes.Link{From: from, To: to, Mode: mode}
	s.Topology.Links = append(s.Topology.Links, link)
	return link
}
func (s *Scenario) transfer(from, to string, duration time.Duration) {
	link := s.link(from, to, routes.Transfer)
	s.Schedule.Transfers = append(s.Schedule.Transfers, journeys.TransferTiming{Link: link, Source: s.Schedule.Source, ObservedAt: observed(), Known: true, Duration: duration})
}
func observed() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }
func at(day, hour, minute, offset int) time.Time {
	return time.Date(2027, 1, day, hour, minute, 0, 0, time.FixedZone("", offset*3600))
}
func (s *Scenario) offer(id string, link routes.Link, departure, arrival time.Time, adults int, currency string) {
	s.Schedule.Offers = append(s.Schedule.Offers, journeys.Offer{ID: id, Source: s.Schedule.Source, Fare: "synthetic-fare", Link: link, Departure: departure, Arrival: arrival, ObservedAt: observed(), Adults: adults, Price: &journeys.Money{MinorUnits: 100000, Currency: currency}})
}
func kazanSochi() Scenario {
	s := scenario("kazan-sochi", "Казань", "Europe/Moscow", "Сочи", "Europe/Moscow", "2027-01-10", 2)
	origin := s.node("departure-airport", s.Query.Route.OriginCityID, routes.Airport)
	destination := s.node("arrival-airport", s.Query.Route.DestinationCityID, routes.Airport)
	s.transfer(s.Query.Route.OriginCityID, origin, 30*time.Minute)
	flight := s.link(origin, destination, routes.Flight)
	s.transfer(destination, s.Query.Route.DestinationCityID, 30*time.Minute)
	s.offer("two-adults", flight, at(10, 12, 0, 3), at(10, 14, 0, 3), 2, "RUB")
	s.offer("one-adult-only", flight, at(10, 11, 0, 3), at(10, 13, 0, 3), 1, "RUB")
	return s
}
func tverDubai() Scenario {
	s := scenario("tver-dubai", "Тверь", "Europe/Moscow", "Дубай", "Asia/Dubai", "2027-01-10", 1)
	hub := s.hub("Москва", "Europe/Moscow")
	start := s.node("origin-station", s.Query.Route.OriginCityID, routes.Station)
	station := s.node("hub-station", hub, routes.Station)
	airport := s.node("hub-airport", hub, routes.Airport)
	end := s.node("destination-airport", s.Query.Route.DestinationCityID, routes.Airport)
	s.transfer(s.Query.Route.OriginCityID, start, 30*time.Minute)
	train := s.link(start, station, routes.Train)
	s.transfer(station, airport, time.Hour)
	flight := s.link(airport, end, routes.Flight)
	s.transfer(end, s.Query.Route.DestinationCityID, 30*time.Minute)
	s.offer("train", train, at(10, 6, 0, 3), at(10, 8, 0, 3), 1, "RUB")
	s.offer("too-short", flight, at(10, 10, 30, 3), at(10, 16, 30, 4), 1, "AED")
	s.offer("valid-flight", flight, at(10, 11, 0, 3), at(10, 17, 0, 4), 1, "AED")
	return s
}
func omskIstanbul() Scenario {
	s := scenario("omsk-istanbul", "Омск", "Asia/Omsk", "Стамбул", "Europe/Istanbul", "2027-01-10", 1)
	hub := s.hub("Москва", "Europe/Moscow")
	start := s.node("local-airport", s.Query.Route.OriginCityID, routes.Airport)
	arrival := s.node("hub-domestic-arrival", hub, routes.Airport)
	departure := s.node("hub-international-departure", hub, routes.Airport)
	end := s.node("destination-airport", s.Query.Route.DestinationCityID, routes.Airport)
	s.transfer(s.Query.Route.OriginCityID, start, 30*time.Minute)
	feeder := s.link(start, arrival, routes.Flight)
	s.transfer(arrival, departure, 90*time.Minute)
	flight := s.link(departure, end, routes.Flight)
	s.transfer(end, s.Query.Route.DestinationCityID, 30*time.Minute)
	s.offer("feeder", feeder, at(10, 8, 0, 6), at(10, 9, 0, 3), 1, "RUB")
	s.offer("too-short", flight, at(10, 12, 0, 3), at(10, 16, 0, 3), 1, "RUB")
	s.offer("valid-flight", flight, at(10, 13, 0, 3), at(10, 17, 0, 3), 1, "RUB")
	return s
}
func tokyoHonolulu() Scenario {
	s := scenario("tokyo-honolulu", "Токио", "Asia/Tokyo", "Гонолулу", "Pacific/Honolulu", "2027-01-11", 1)
	start := s.node("origin-airport", s.Query.Route.OriginCityID, routes.Airport)
	end := s.node("destination-airport", s.Query.Route.DestinationCityID, routes.Airport)
	s.transfer(s.Query.Route.OriginCityID, start, 30*time.Minute)
	flight := s.link(start, end, routes.Flight)
	s.transfer(end, s.Query.Route.DestinationCityID, 30*time.Minute)
	s.offer("previous-local-arrival-date", flight, at(11, 8, 0, 9), at(10, 20, 0, -10), 1, "JPY")
	// Flight leaves on the requested day, but departure from the city is the day before.
	s.offer("city-start-too-early", flight, at(11, 0, 30, 9), at(10, 12, 30, -10), 1, "JPY")
	return s
}
