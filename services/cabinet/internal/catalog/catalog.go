// Package catalog imports geographic reference data independently of its source.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

type Country struct{ Code, Name string }
type City struct {
	SourceID                                int64
	Name, CountryCode, RegionCode, Timezone string
	Aliases                                 []string
	Latitude, Longitude                     float64
	Population                              int64
}
type Snapshot struct {
	Source    string
	Version   string // Content digest, not a claim about the source's publication date.
	Countries []Country
	Cities    []City
}

// Source implementations must respect cancellation and bound input sizes.
type Source interface {
	Load(context.Context) (Snapshot, error)
}

// Repository publishes the entire snapshot atomically, preserving city identities.
type Repository interface {
	ReplaceCatalog(context.Context, Snapshot) error
}

type Importer struct {
	Source     Source
	Repository Repository
	MinCities  int
}

func (i Importer) Run(ctx context.Context) (int, error) {
	snapshot, err := i.Source.Load(ctx)
	if err != nil {
		return 0, fmt.Errorf("load city catalog: %w", err)
	}
	if err := Validate(snapshot, i.MinCities); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := i.Repository.ReplaceCatalog(ctx, snapshot); err != nil {
		return 0, err
	}
	return len(snapshot.Cities), nil
}
func Validate(s Snapshot, minCities int) error {
	if minCities < 1 || len(s.Cities) < minCities || s.Source == "" || s.Version == "" {
		return errors.New("incomplete city catalog")
	}
	countries := make(map[string]bool, len(s.Countries))
	for _, c := range s.Countries {
		if len(c.Code) != 2 || c.Code[0] < 'A' || c.Code[0] > 'Z' || c.Code[1] < 'A' || c.Code[1] > 'Z' || !validText(c.Name, 200) || countries[c.Code] {
			return errors.New("invalid or duplicate country")
		}
		countries[c.Code] = true
	}
	ids := make(map[int64]bool, len(s.Cities))
	zones := make(map[string]bool)
	for _, c := range s.Cities {
		if c.SourceID <= 0 || ids[c.SourceID] || !countries[c.CountryCode] || !validText(c.Name, 200) || c.Population < 0 || !(c.Latitude >= -90 && c.Latitude <= 90) || !(c.Longitude >= -180 && c.Longitude <= 180) {
			return errors.New("invalid or duplicate city")
		}
		if c.Timezone == "" || c.Timezone == "Local" {
			return errors.New("invalid city timezone")
		}
		if !zones[c.Timezone] {
			if _, err := time.LoadLocation(c.Timezone); err != nil {
				return errors.New("invalid city timezone")
			}
			zones[c.Timezone] = true
		}
		if len(c.RegionCode) > 100 || !utf8.ValidString(c.RegionCode) {
			return errors.New("invalid city region")
		}
		for _, alias := range c.Aliases {
			if !validText(alias, 400) {
				return errors.New("invalid city alias")
			}
		}
		ids[c.SourceID] = true
	}
	return nil
}
func validText(s string, max int) bool {
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if r < 32 {
			return false
		}
	}
	return true
}
