package journeys

import (
	"context"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	"strings"
	"time"
)

func validate(ctx context.Context, topology routes.Snapshot, s Snapshot, q Query, p Policy, l Limits) (time.Time, time.Time, error) {
	invalid := func() (time.Time, time.Time, error) { return time.Time{}, time.Time{}, ErrInvalidInput }
	if s.Source == "" || q.Adults < 1 || q.Adults > 9 || l.MaxOffers < 1 || l.MaxTransfers < 1 || l.MaxCombinations < 1 || l.MaxJourneys < 1 {
		return invalid()
	}
	// Bound arithmetic to prevent duration overflow; these are safety bounds, not defaults.
	for _, d := range []time.Duration{p.TrainBoarding, p.FlightBoarding, p.TrainExit, p.FlightExit, p.DelayReserve} {
		if d < 0 || d > 24*time.Hour {
			return invalid()
		}
	}
	if p.MaxConnection <= 0 || p.MaxConnection > 30*24*time.Hour || p.MaxJourney <= 0 || p.MaxJourney > 365*24*time.Hour {
		return invalid()
	}
	zone := ""
	for _, city := range topology.Cities {
		if city.ID == q.Route.OriginCityID {
			zone = city.Timezone
		}
	}
	if zone == "" {
		return invalid()
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return invalid()
	}
	from, err := time.ParseInLocation(time.DateOnly, q.DepartureFrom, location)
	if err != nil {
		return invalid()
	}
	until, err := time.ParseInLocation(time.DateOnly, q.DepartureTo, location)
	if err != nil || until.Before(from) {
		return invalid()
	}
	links := make(map[routes.Link]bool)
	for _, link := range topology.Links {
		links[link] = true
	}
	identities := make(map[[2]string]bool)
	for _, o := range s.Offers {
		if err := ctx.Err(); err != nil {
			return time.Time{}, time.Time{}, err
		}
		id := [2]string{o.Source, o.ID}
		if o.ID == "" || o.Source == "" || identities[id] || !links[o.Link] || o.Link.Mode == routes.Transfer || o.Adults < 1 || o.Adults > 9 || o.Departure.IsZero() || o.Arrival.IsZero() || !o.Arrival.After(o.Departure) || o.ObservedAt.IsZero() {
			return invalid()
		}
		if o.Price != nil && (o.Price.MinorUnits < 0 || len(o.Price.Currency) != 3 || strings.IndexFunc(o.Price.Currency, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0) {
			return invalid()
		}
		identities[id] = true
	}
	seen := make(map[routes.Link]bool)
	for _, t := range s.Transfers {
		if err := ctx.Err(); err != nil {
			return time.Time{}, time.Time{}, err
		}
		if seen[t.Link] || !links[t.Link] || t.Link.Mode != routes.Transfer || t.Source == "" || t.ObservedAt.IsZero() || t.Duration < 0 || t.Duration > 7*24*time.Hour || !t.Known && t.Duration != 0 {
			return invalid()
		}
		seen[t.Link] = true
	}
	return from, until.AddDate(0, 0, 1), nil
}
