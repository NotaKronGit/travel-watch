package journeys

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
)

var ErrInvalidInput = errors.New("invalid schedule input or policy")
var ErrTooLarge = errors.New("schedule input exceeds limits")

// Assemble uses no network or database. Its caller supplies one bounded snapshot.
func Assemble(ctx context.Context, topology routes.Snapshot, candidates routes.Result, data Snapshot, q Query, p Policy, l Limits) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if len(data.Offers) > l.MaxOffers || len(data.Transfers) > l.MaxTransfers {
		return Result{}, ErrTooLarge
	}
	from, to, err := validate(ctx, topology, data, q, p, l)
	if err != nil {
		return Result{}, err
	}
	candidates, err = routes.ValidateResult(ctx, topology, q.Route, l.Routes, candidates)
	if err != nil {
		return Result{}, err
	}
	r := Result{Source: data.Source, Synthetic: topology.Synthetic || data.Synthetic, InputIncomplete: !data.Complete || !candidates.Complete(), Journeys: []Journey{}}
	transfers := make(map[routes.Link]TransferTiming)
	for _, v := range data.Transfers {
		transfers[v.Link] = v
	}
	offers := make(map[routes.Link][]Offer)
	for _, o := range data.Offers {
		if o.Adults == q.Adults {
			offers[o.Link] = append(offers[o.Link], o)
		}
	}
	for link, list := range offers {
		slices.SortFunc(list, func(a, b Offer) int {
			return cmp.Or(a.Departure.Compare(b.Departure), cmp.Compare(a.Source, b.Source), cmp.Compare(a.ID, b.ID))
		})
		offers[link] = list
	}
	steps := 0
	for _, c := range candidates.Candidates {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if r.LimitReached {
			break
		}
		known := true
		for _, leg := range c.Legs {
			if leg.Mode == routes.Transfer && !transfers[leg].Known {
				known = false
				r.MissingTiming = true
			}
		}
		if !known {
			continue
		}
		var visit func(int, []Offer) error
		visit = func(index int, selected []Offer) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if index == len(c.Legs) {
				j, ok := combine(c, selected, transfers, p)
				if !ok || j.Start.Before(from) || !j.Start.Before(to) || j.End.Sub(j.Start) > p.MaxJourney {
					return nil
				}
				if len(r.Journeys) == l.MaxJourneys {
					r.LimitReached = true
					return nil
				}
				r.Journeys = append(r.Journeys, j)
				return nil
			}
			link := c.Legs[index]
			if link.Mode == routes.Transfer {
				return visit(index+1, selected)
			}
			for _, o := range offers[link] {
				if r.LimitReached {
					return nil
				}
				if steps == l.MaxCombinations {
					r.LimitReached = true
					return nil
				}
				steps++
				if err := visit(index+1, append(selected, o)); err != nil {
					return err
				}
			}
			return nil
		}
		if err := visit(0, nil); err != nil {
			return Result{}, err
		}
	}
	return r, nil
}
func boarding(mode routes.Mode, p Policy) time.Duration {
	if mode == routes.Flight {
		return p.FlightBoarding
	}
	return p.TrainBoarding
}
func exitTime(mode routes.Mode, p Policy) time.Duration {
	if mode == routes.Flight {
		return p.FlightExit
	}
	return p.TrainExit
}
func combine(c routes.Candidate, offers []Offer, timings map[routes.Link]TransferTiming, p Policy) (Journey, bool) {
	first := offers[0]
	start := first.Departure.Add(-boarding(first.Link.Mode, p)).Add(-timings[c.Legs[0]].Duration)
	j := Journey{Start: start}
	clock := start
	n := 0
	for _, link := range c.Legs {
		if link.Mode == routes.Transfer {
			end := clock.Add(timings[link].Duration)
			j.Legs = append(j.Legs, Leg{Link: link, Start: clock, End: end})
			clock = end
			continue
		}
		offer := offers[n]
		if n > 0 {
			previous := offers[n-1]
			gap := offer.Departure.Sub(previous.Arrival)
			if gap > p.MaxConnection || offer.Departure.Before(clock.Add(p.DelayReserve).Add(boarding(link.Mode, p))) {
				return Journey{}, false
			}
		}
		copied := offer
		if offer.Price != nil {
			price := *offer.Price
			copied.Price = &price
		}
		j.Legs = append(j.Legs, Leg{Link: link, Start: offer.Departure, End: offer.Arrival, Offer: &copied})
		clock = offer.Arrival.Add(exitTime(link.Mode, p))
		n++
	}
	j.End = clock
	return j, true
}
