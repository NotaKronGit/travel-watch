// Package airports imports airport reference data, not flight routes.
package airports

import (
	"context"
	"errors"
	"math"
	"regexp"
	"time"
)

type Airport struct {
	SourceID     int64   `json:"source_id"`
	Ident        string  `json:"ident"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Country      string  `json:"country"`
	Region       string  `json:"region"`
	Municipality string  `json:"municipality"`
	IATA         string  `json:"iata"`
	ICAO         string  `json:"icao"`
	Scheduled    bool    `json:"scheduled_service"`
}
type Snapshot struct {
	FetchedAt time.Time
	Airports  []Airport
}
type Source interface {
	Load(context.Context) (Snapshot, error)
}
type Repository interface {
	PublishAirports(context.Context, Snapshot, int) error
}
type Importer struct {
	Source                               Source
	Repository                           Repository
	MinRows, MaxRows, MinRetainedPercent int
}

func (i Importer) Run(ctx context.Context) (int, error) {
	if i.Source == nil || i.Repository == nil || i.MinRows < 1 || i.MaxRows < i.MinRows || i.MinRetainedPercent < 1 || i.MinRetainedPercent > 100 {
		return 0, errors.New("invalid airport import configuration")
	}
	s, err := i.Source.Load(ctx)
	if err != nil {
		return 0, err
	}
	if err = Validate(ctx, s, i.MinRows, i.MaxRows); err != nil {
		return 0, err
	}
	if err = i.Repository.PublishAirports(ctx, s, i.MinRetainedPercent); err != nil {
		return 0, err
	}
	return len(s.Airports), nil
}

var codePattern = regexp.MustCompile(`^[A-Z]{3}$`)

func Validate(ctx context.Context, s Snapshot, minRows, maxRows int) error {
	if s.FetchedAt.IsZero() || len(s.Airports) < minRows || len(s.Airports) > maxRows {
		return errors.New("airport snapshot size or timestamp invalid")
	}
	ids := map[int64]bool{}
	idents := map[string]bool{}
	for _, a := range s.Airports {
		if err := ctx.Err(); err != nil {
			return err
		}
		if a.SourceID < 1 || ids[a.SourceID] || a.Ident == "" || idents[a.Ident] || a.Name == "" || len(a.Country) != 2 || math.IsNaN(a.Latitude) || math.IsNaN(a.Longitude) || math.IsInf(a.Latitude, 0) || math.IsInf(a.Longitude, 0) || a.Latitude < -90 || a.Latitude > 90 || a.Longitude < -180 || a.Longitude > 180 || (a.IATA != "" && !codePattern.MatchString(a.IATA)) {
			return errors.New("invalid or duplicate airport record")
		}
		switch a.Type {
		case "small_airport", "medium_airport", "large_airport", "heliport", "seaplane_base", "balloonport", "closed":
		default:
			return errors.New("unknown airport type")
		}
		for _, v := range []string{a.Ident, a.Name, a.Country, a.Region, a.Municipality, a.IATA, a.ICAO} {
			if len(v) > 1000 {
				return errors.New("airport field too long")
			}
		}
		ids[a.SourceID] = true
		idents[a.Ident] = true
	}
	return nil
}
