package schedules

import (
	"context"
	"math/rand"
	"testing"
	"time"

	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"google.golang.org/protobuf/proto"
)

func synthDeparture(departure, arrival time.Time) observed {
	return observed{timetableDeparture: timetableDeparture{Departure: departure, Arrival: arrival, Mode: "plane", Number: "X"}, at: departure}
}
func journeysEqual(a, b []*v1.ScheduledJourney) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !proto.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// randomScenario builds a chain of 1-legs legs, each with a random count of
// randomly timed departures over a fixed window. Modes alternate so both
// before/after policies get exercised. Limits stay within Config.Validate's
// bounds and generous enough that neither matcher truncates, which is what
// the differential test below relies on.
func randomScenario(rng *rand.Rand, legs int) matchInput {
	base := stamp("2027-03-01T00:00:00Z")
	steps := make([]realroutes.Step, legs)
	main := make([]int, legs)
	available := make([][]observed, legs)
	modes := []string{"train", "plane"}
	for i := range legs {
		steps[i] = realroutes.Step{Mode: modes[rng.Intn(len(modes))]}
		main[i] = i
		count := 1 + rng.Intn(10)
		deps := make([]observed, count)
		for j := range deps {
			start := base.Add(time.Duration(rng.Intn(10*24*60)) * time.Minute)
			dur := time.Duration(30+rng.Intn(300)) * time.Minute
			deps[j] = synthDeparture(start, start.Add(dur))
		}
		available[i] = deps
	}
	cfg := policy()
	cfg.MaxCombinations = 100000
	cfg.MaxJourneys = 5
	cfg.MaxJourneysPerDay = rng.Intn(3) // 0 disables the per-day limit
	return matchInput{Steps: steps, Main: main, Transfers: map[int]time.Duration{}, Available: available, From: base, End: base.Add(10 * 24 * time.Hour), Config: cfg}
}

// TestWindowedMatchesBruteForce checks that narrowing candidates ahead of the
// search never changes which journeys are found or reported as truncated,
// only how many dead branches are visited to find them. Dead branches (ones
// narrowWindows removes) never contribute a journey, so both matchers walk
// live branches in the same relative order and must reach MaxJourneys, or
// run out of input, at the same point. MaxCombinations is set generous
// enough (given the scenario sizes below) that it never triggers here — if
// it did, windowed could legitimately find more than brute, since it does
// provably no more work per candidate than brute does; that is a separate,
// intentionally untested property. Random, but seeded for reproducibility.
func TestWindowedMatchesBruteForce(t *testing.T) {
	for seed := range int64(300) {
		rng := rand.New(rand.NewSource(seed)) //nolint:gosec // G404: seeded for reproducible test scenarios; not used for security.
		in := randomScenario(rng, 1+rng.Intn(4))
		brute, bruteTruncated, bruteAttempts := searchCounting(context.Background(), in, in.Available)
		windowed, windowedTruncated, _ := searchCounting(context.Background(), in, narrowWindows(in))
		if bruteAttempts > in.Config.MaxCombinations {
			t.Fatalf("seed %d: scenario exceeds the combinations budget; MaxCombinations is no longer generous enough for this test's assumptions", seed)
		}
		if bruteTruncated != windowedTruncated {
			t.Fatalf("seed %d: truncation disagrees (brute=%v windowed=%v)", seed, bruteTruncated, windowedTruncated)
		}
		if !journeysEqual(brute, windowed) {
			t.Fatalf("seed %d: matchers disagree\nbrute:    %+v\nwindowed: %+v", seed, brute, windowed)
		}
	}
}

// TestWindowedVisitsFewerCombinationsWhenMostAreIrrelevant is the concrete
// motivating case: many departures on the first leg, only a handful of which
// could ever reach any departure of the second leg.
func TestWindowedVisitsFewerCombinationsWhenMostAreIrrelevant(t *testing.T) {
	base := stamp("2027-03-01T00:00:00Z")
	trains := make([]observed, 50)
	for i := range trains {
		start := base.Add(time.Duration(i) * 3 * time.Hour)
		trains[i] = synthDeparture(start, start.Add(2*time.Hour))
	}
	flights := []observed{
		synthDeparture(base.Add(40*time.Hour+30*time.Minute), base.Add(43*time.Hour+30*time.Minute)),
		synthDeparture(base.Add(41*time.Hour), base.Add(44*time.Hour)),
		synthDeparture(base.Add(41*time.Hour+30*time.Minute), base.Add(44*time.Hour+30*time.Minute)),
		synthDeparture(base.Add(42*time.Hour), base.Add(45*time.Hour)),
		synthDeparture(base.Add(42*time.Hour+30*time.Minute), base.Add(45*time.Hour+30*time.Minute)),
	}
	cfg := policy()
	cfg.MaxCombinations = 100000
	cfg.MaxJourneys = 5
	in := matchInput{
		Steps:     []realroutes.Step{{Mode: "train"}, {Mode: "plane"}},
		Main:      []int{0, 1},
		Transfers: map[int]time.Duration{},
		Available: [][]observed{trains, flights},
		From:      base,
		End:       base.Add(6 * 24 * time.Hour),
		Config:    cfg,
	}
	brute, _, bruteAttempts := searchCounting(context.Background(), in, in.Available)
	windowed, _, windowedAttempts := searchCounting(context.Background(), in, narrowWindows(in))
	if !journeysEqual(brute, windowed) {
		t.Fatalf("matchers disagree: brute=%+v windowed=%+v", brute, windowed)
	}
	if windowedAttempts >= bruteAttempts {
		t.Fatalf("windowed matcher did not reduce visited combinations: brute=%d windowed=%d", bruteAttempts, windowedAttempts)
	}
	filtered := len(narrowWindows(in)[0])
	t.Logf("leg 0 candidates: %d -> %d after filtering; combinations visited: brute=%d windowed=%d (%.0f%% fewer)",
		len(trains), filtered, bruteAttempts, windowedAttempts, 100*(1-float64(windowedAttempts)/float64(bruteAttempts)))
}
