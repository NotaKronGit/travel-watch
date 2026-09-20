package realroutes

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
)

// Dates are discovery samples, never a claim that a feeder train and flight connect.
func sampleDates(q Query) ([]string, error) {
	from, err := time.Parse(time.DateOnly, q.DepartureFrom)
	if err != nil {
		return nil, errors.New("flight discovery requires departure dates")
	}
	to, err := time.Parse(time.DateOnly, q.DepartureTo)
	if err != nil || to.Before(from) || to.Sub(from) > 366*24*time.Hour || q.Adults < 1 || q.Adults > 9 {
		return nil, errors.New("invalid flight discovery dates or passengers")
	}
	days := int(to.Sub(from) / (24 * time.Hour))
	dates := []string{q.DepartureFrom}
	if days > 1 {
		dates = append(dates, from.AddDate(0, 0, days/2).Format(time.DateOnly))
	}
	if days > 0 {
		dates = append(dates, q.DepartureTo)
	}
	return dates, nil
}

type flightPair struct {
	from, to airports.Airport
	paths    []Candidate
	seen     map[string]int
	accesses [][]Step
}

func (s *run) googlePaths(ctx context.Context, q Query, origin, dest, hubs []airports.Airport, cityCode string) {
	dates, _ := sampleDates(q)
	s.r.Source = "Yandex Rasp + Google Flights/Fli + OurAirports"
	s.issue("Google Flights samples departure dates; route coverage and timetable compatibility are incomplete")
	departures := diverseAirports(append(append([]airports.Airport(nil), origin...), hubs...), q.Origin)
	pairs := []flightPair{}
	// All departure hubs get the first destination before a second destination.
	for _, b := range dest {
		for _, a := range departures {
			if a.IATA != b.IATA {
				pairs = append(pairs, flightPair{from: a, to: b, seen: map[string]int{}})
			}
		}
	}
	requests := 0
discovery:
	for _, date := range dates {
		for i := range pairs {
			if ctx.Err() != nil {
				break discovery
			}
			if requests >= s.p.Config.GoogleFlights.MaxRequests {
				s.r.LimitReached = true
				s.issue("Google Flights request budget reached")
				break discovery
			}
			pair := &pairs[i]
			requests++
			s.flightRequests++
			s.r.Requests++
			reply, err := s.p.Provider.Call(ctx, transport.Request{Method: "flight-paths", From: pair.from.IATA, To: pair.to.IATA, Date: date, Adults: q.Adults, Limit: s.p.Config.GoogleFlights.MaxOptions})
			check := FlightCheck{From: pair.from.IATA, To: pair.to.IATA, Date: date, Outcome: "observed"}
			if err != nil {
				check.Outcome = "failed"
				s.r.ProviderFailures++
				s.issue("Google Flights request failed")
			} else if len(reply.FlightPaths) > s.p.Config.GoogleFlights.MaxOptions {
				check.Outcome = "failed"
				s.r.ProviderFailures++
				s.issue("Invalid Google Flights route response")
			} else {
				if len(reply.FlightPaths) == 0 {
					check.Outcome = "empty"
				}
				for _, path := range reply.FlightPaths {
					if !validFlightPath(path, pair.from.IATA, pair.to.IATA) {
						check.Outcome = "failed"
						s.r.ProviderFailures++
						s.issue("Invalid Google Flights route response")
						continue
					}
					steps := []Step{}
					keyParts := []string{}
					for j, leg := range path.Legs {
						if j > 0 && path.Legs[j-1].To.IATA != leg.From.IATA {
							steps = append(steps, transfer(s.airportName(path.Legs[j-1].To.IATA), s.airportName(leg.From.IATA)))
						}
						keyParts = append(keyParts, leg.From.IATA, leg.To.IATA)
						steps = append(steps, Step{FromCode: "iata:" + leg.From.IATA, ToCode: "iata:" + leg.To.IATA, From: s.airportName(leg.From.IATA), To: s.airportName(leg.To.IATA), Mode: "plane", Evidence: "google-flights-fli", ObservedDates: []string{date}})
					}
					encoded, _ := json.Marshal(keyParts)
					key := string(encoded)
					if index, ok := pair.seen[key]; ok {
						for j := range pair.paths[index].Steps {
							step := &pair.paths[index].Steps[j]
							if step.Evidence == "google-flights-fli" && step.ObservedDates[len(step.ObservedDates)-1] != date {
								step.ObservedDates = append(step.ObservedDates, date)
							}
						}
					} else {
						pair.seen[key] = len(pair.paths)
						pair.paths = append(pair.paths, Candidate{Steps: steps})
					}
				}
			}
			s.r.FlightChecks = append(s.r.FlightChecks, check)
		}
	}
	// Resolve feeder access only for observed chains, once per departure airport.
	accessCache := map[string][][]Step{}
	for i := range pairs {
		pair := &pairs[i]
		if len(pair.paths) == 0 {
			continue
		}
		access, ok := accessCache[pair.from.IATA]
		if !ok {
			access = s.flightAccess(ctx, q, pair.from, origin, cityCode, true)
			accessCache[pair.from.IATA] = access
		}
		pair.accesses = access
	}
	// Preserve diversity across pairs when the unique-scheme cap is small.
	for round := 0; ; round++ {
		added := false
		for _, pair := range pairs {
			if round >= len(pair.paths) {
				continue
			}
			added = true
			for _, access := range pair.accesses {
				steps := append(append([]Step(nil), access...), pair.paths[round].Steps...)
				steps = append(steps, transfer(s.airportName(pair.to.IATA), q.DestinationName))
				s.add(steps)
			}
		}
		if !added {
			break
		}
	}
}
func (s *run) airportName(iata string) string {
	for _, a := range s.p.Airports {
		if a.IATA == iata {
			return a.Name + " (" + iata + ")"
		}
	}
	return iata
}
func validFlightPath(p transport.FlightPath, from, to string) bool {
	if len(p.Legs) == 0 || len(p.Legs) > 8 || p.Legs[0].From.IATA != from || p.Legs[len(p.Legs)-1].To.IATA != to {
		return false
	}
	for _, l := range p.Legs {
		if !validIATA(l.From.IATA) || !validIATA(l.To.IATA) || l.From.IATA == l.To.IATA || l.Mode != "plane" {
			return false
		}
	}
	return true
}
func validIATA(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}
func (s *run) flightAccess(ctx context.Context, q Query, a airports.Airport, origin []airports.Airport, cityCode string, needed bool) [][]Step {
	if !needed {
		return nil
	}
	target := s.airportName(a.IATA)
	if distance(q.Origin, point(a)) <= s.p.Config.AirportRadiusKM {
		return [][]Step{{transfer(q.OriginName, target)}}
	}
	result := [][]Step{}
	if cityCode != "" {
		pt := point(a)
		stations, ok := s.call(ctx, transport.Request{Method: "stations", Center: &pt, Radius: s.p.Config.RailRadiusKM})
		if ok {
			for _, station := range stations.Stations {
				for _, train := range s.search(ctx, cityCode, station.Code, "yandex", "train") {
					result = append(result, []Step{transfer(q.OriginName, train.From.Title), observed(train), transfer(train.To.Title, target)})
				}
			}
		}
	}
	for _, arrival := range s.near(point(a), s.p.Config.RailRadiusKM) {
		for _, departure := range origin {
			if departure.IATA == arrival.IATA {
				continue
			}
			for _, flight := range s.search(ctx, departure.IATA, arrival.IATA, "iata", "plane") {
				steps := []Step{transfer(q.OriginName, flight.From.Title), observed(flight)}
				// Keep explicit transfer even when catalogue/provider labels differ.
				if flight.To.Title != target {
					steps = append(steps, transfer(flight.To.Title, target))
				}
				result = append(result, steps)
			}
		}
	}
	return result
}
