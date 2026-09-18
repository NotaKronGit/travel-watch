// Package flights defines provider-neutral, date-specific flight lookups.
package flights

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var (
	ErrInvalidQuery    = errors.New("invalid flight query")
	ErrUnavailable     = errors.New("flight provider unavailable")
	ErrRateLimited     = errors.New("flight provider rate limited")
	ErrUnsupported     = errors.New("flight query unsupported")
	ErrInvalidResponse = errors.New("invalid flight provider response")
)
var iata = regexp.MustCompile(`^[A-Z]{3}$`)

type Query struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Date    string `json:"date"`
	Adults  int    `json:"adults"`
	Limit   int    `json:"limit"`
	Nonstop bool   `json:"nonstop"`
}

func (q Query) Validate() error {
	d, err := time.Parse(time.DateOnly, q.Date)
	if err != nil || d.Format(time.DateOnly) != q.Date || !iata.MatchString(q.From) || !iata.MatchString(q.To) || q.From == q.To || q.Adults < 1 || q.Adults > 9 || q.Limit < 1 || q.Limit > 50 {
		return ErrInvalidQuery
	}
	return nil
}

// Local times intentionally have no invented UTC offset. Timetable matching
// must resolve airport timezones before using these values as instants.
type Leg struct {
	From           string `json:"from"`
	To             string `json:"to"`
	Number         string `json:"number"`
	DepartureLocal string `json:"departure_local"`
	ArrivalLocal   string `json:"arrival_local"`
}
type Option struct {
	Legs            []Leg `json:"legs"`
	DurationMinutes int   `json:"duration_minutes"`
}
type Result struct {
	Provider     string    `json:"provider"`
	ObservedAt   time.Time `json:"observed_at"`
	Query        Query     `json:"query"`
	Complete     bool      `json:"complete"`
	LimitReached bool      `json:"limit_reached"`
	Options      []Option  `json:"options"`
	Warnings     []string  `json:"warnings"`
}
type Provider interface {
	FindFlights(context.Context, Query) (Result, error)
}

func (r Result) Validate(q Query) error {
	if r.Query != q || r.Provider == "" || r.ObservedAt.IsZero() || len(r.Options) > q.Limit || (r.LimitReached && r.Complete) {
		return ErrInvalidResponse
	}
	for _, o := range r.Options {
		if len(o.Legs) == 0 || len(o.Legs) > 8 || o.DurationMinutes <= 0 || (q.Nonstop && len(o.Legs) != 1) || o.Legs[0].From != q.From || o.Legs[len(o.Legs)-1].To != q.To {
			return ErrInvalidResponse
		}
		for i, l := range o.Legs {
			d, err := time.Parse("2006-01-02T15:04:05", l.DepartureLocal)
			if err != nil {
				return ErrInvalidResponse
			}
			if _, err = time.Parse("2006-01-02T15:04:05", l.ArrivalLocal); err != nil {
				return ErrInvalidResponse
			}
			if !iata.MatchString(l.From) || !iata.MatchString(l.To) || l.From == l.To || l.Number == "" || len(l.Number) > 32 || (i == 0 && d.Format(time.DateOnly) != q.Date) {
				return ErrInvalidResponse
			}
		}
	}
	return nil
}
