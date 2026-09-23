// Package schedules checks saved physical route schemes against dated timetables.
package schedules

import (
	"errors"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
)

// Transfer is a known directed ground transfer between endpoint codes
// (city:<id>, Yandex station code or iata:<IATA>), never between titles.
type Transfer struct {
	From     string        `mapstructure:"from"`
	To       string        `mapstructure:"to"`
	Duration time.Duration `mapstructure:"duration"`
	Source   string        `mapstructure:"source"`
}
type Config struct {
	Enabled         bool          `mapstructure:"enabled"`
	Timeout         time.Duration `mapstructure:"timeout"`
	MaxRequests     int           `mapstructure:"max_requests"`
	MaxDates        int           `mapstructure:"max_dates"`
	MaxCombinations int           `mapstructure:"max_combinations"`
	MaxJourneys     int           `mapstructure:"max_journeys"`
	BeforeTrain     time.Duration `mapstructure:"before_train"`
	AfterTrain      time.Duration `mapstructure:"after_train"`
	BeforePlane     time.Duration `mapstructure:"before_plane"`
	AfterPlane      time.Duration `mapstructure:"after_plane"`
	Buffer          time.Duration `mapstructure:"buffer"`
	MaxConnection   time.Duration `mapstructure:"max_connection"`
	MaxJourney      time.Duration `mapstructure:"max_journey"`
	Transfers       []Transfer    `mapstructure:"transfers"`
	// Matcher selects the combination search strategy: "windowed" (default,
	// also used when empty) prunes each leg's candidates by a feasible time
	// window before searching; "brute" searches every fetched departure
	// without pruning. Both produce identical results (see match_test.go);
	// windowed does less work when a leg's schedule is far outside any
	// useful window (e.g. no connecting service exists at all), at the cost
	// of the pruning pass itself, which can outweigh the saving on small,
	// already-cheap schemes.
	Matcher string `mapstructure:"matcher"`
}

func (c Config) Validate() error {
	if c.Timeout < time.Second || c.Timeout > 15*time.Minute || c.MaxRequests < 1 || c.MaxRequests > 300 || c.MaxDates < 1 || c.MaxDates > 31 || c.MaxCombinations < 1 || c.MaxCombinations > 100000 || c.MaxJourneys < 1 || c.MaxJourneys > 50 || c.MaxConnection < time.Hour || c.MaxConnection > 48*time.Hour || c.MaxJourney < c.MaxConnection || c.MaxJourney > 120*time.Hour {
		return errors.New("invalid schedule limits")
	}
	if c.Matcher != "" && c.Matcher != "windowed" && c.Matcher != "brute" {
		return errors.New("invalid schedule matcher")
	}
	for _, d := range []time.Duration{c.BeforeTrain, c.AfterTrain, c.BeforePlane, c.AfterPlane, c.Buffer} {
		if d < 0 || d > 12*time.Hour {
			return errors.New("invalid schedule buffer")
		}
	}
	seen := map[[2]string]bool{}
	for _, t := range c.Transfers {
		k := [2]string{t.From, t.To}
		if !realroutes.ValidEndpointCode(t.From) || !realroutes.ValidEndpointCode(t.To) || t.Source == "" || t.Duration < 0 || t.Duration > 24*time.Hour || seen[k] {
			return errors.New("invalid schedule transfer")
		}
		seen[k] = true
	}
	return nil
}
func (c Config) before(mode string) time.Duration {
	if mode == "train" {
		return c.BeforeTrain
	}
	return c.BeforePlane
}
func (c Config) after(mode string) time.Duration {
	if mode == "train" {
		return c.AfterTrain
	}
	return c.AfterPlane
}
