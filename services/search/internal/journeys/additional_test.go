package journeys_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	test "github.com/NotaKronGit/travel-watch/services/search/internal/journeys/testsource"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
)

func TestAdditionalRoutes(t *testing.T) {
	expected := map[string]struct {
		ids      []string
		duration time.Duration
	}{
		"kazan-sochi":    {[]string{"two-adults"}, 4*time.Hour + 30*time.Minute},
		"tver-dubai":     {[]string{"train", "valid-flight"}, 11*time.Hour + 40*time.Minute},
		"omsk-istanbul":  {[]string{"feeder", "valid-flight"}, 14*time.Hour + 30*time.Minute},
		"tokyo-honolulu": {[]string{"previous-local-arrival-date"}, 9*time.Hour + 30*time.Minute},
	}
	for _, scenario := range test.AdditionalRoutes() {
		t.Run(scenario.Name, func(t *testing.T) {
			ctx := context.Background()
			candidates, err := (routes.GraphPlanner{}).Plan(ctx, scenario.Topology, scenario.Query.Route, limits.Routes)
			if err != nil {
				t.Fatal(err)
			}
			if len(candidates.Candidates) != 1 {
				t.Fatal("unexpected topology", len(candidates.Candidates))
			}
			r, err := journeys.Assemble(ctx, scenario.Topology, candidates, scenario.Schedule, scenario.Query, scenario.Policy, limits)
			if err != nil {
				t.Fatal(err)
			}
			if !r.Complete() || !r.Synthetic || len(r.Journeys) != 1 {
				t.Fatalf("unexpected result %+v", r)
			}
			j := r.Journeys[0]
			want := expected[scenario.Name]
			ids := []string{}
			for _, leg := range j.Legs {
				if leg.Offer != nil {
					ids = append(ids, leg.Offer.ID)
					if leg.Offer.Adults != scenario.Query.Adults {
						t.Fatal("wrong passenger quote")
					}
				}
			}
			if !slices.Equal(ids, want.ids) {
				t.Fatalf("offers %v, want %v", ids, want.ids)
			}
			if j.End.Sub(j.Start) != want.duration {
				t.Fatalf("duration %s, want %s", j.End.Sub(j.Start), want.duration)
			}
			if scenario.Name == "tokyo-honolulu" {
				o := j.Legs[1].Offer
				if o.Arrival.Format(time.DateOnly) >= o.Departure.Format(time.DateOnly) || !o.Arrival.After(o.Departure) {
					t.Fatal("date-line scenario not exercised")
				}
			}
			// Input order must not affect selected offers or their times.
			slices.Reverse(scenario.Schedule.Offers)
			repeat, err := journeys.Assemble(ctx, scenario.Topology, candidates, scenario.Schedule, scenario.Query, scenario.Policy, limits)
			if err != nil || !reflect.DeepEqual(r, repeat) {
				t.Fatal("order-dependent result", err)
			}
			// Every scenario must distinguish missing ground timing from known impossibility.
			scenario.Schedule.Transfers[0].Known = false
			scenario.Schedule.Transfers[0].Duration = 0
			missing, err := journeys.Assemble(ctx, scenario.Topology, candidates, scenario.Schedule, scenario.Query, scenario.Policy, limits)
			if err != nil || !missing.MissingTiming || missing.Complete() || len(missing.Journeys) != 0 {
				t.Fatal("unknown timing accepted", err)
			}
		})
	}
}
