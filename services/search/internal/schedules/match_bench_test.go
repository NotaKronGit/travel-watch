package schedules

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
)

// benchScenario builds a fixed (non-random-per-run) matchInput so benchmark
// numbers are stable and comparable across runs.
func benchScenario(legs, depsPerLeg int, spread time.Duration) matchInput {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: fixed seed keeps benchmark inputs reproducible; not used for security.
	base := stamp("2027-03-01T00:00:00Z")
	steps := make([]realroutes.Step, legs)
	main := make([]int, legs)
	available := make([][]observed, legs)
	modes := []string{"train", "plane"}
	for i := range legs {
		steps[i] = realroutes.Step{Mode: modes[i%len(modes)]}
		main[i] = i
		deps := make([]observed, depsPerLeg)
		for j := range deps {
			start := base.Add(time.Duration(rng.Int63n(int64(spread))))
			dur := time.Duration(30+rng.Intn(300)) * time.Minute
			deps[j] = synthDeparture(start, start.Add(dur))
		}
		available[i] = deps
	}
	cfg := policy()
	cfg.MaxCombinations = 100000
	cfg.MaxJourneys = 5
	return matchInput{Steps: steps, Main: main, Transfers: map[int]time.Duration{}, Available: available, From: base, End: base.Add(spread), Config: cfg}
}

// manyIrrelevantScenario mirrors the concrete motivating case: 50 candidate
// departures on the first leg, spread across a week, only a handful of which
// could ever reach the second leg's tightly clustered departures.
func manyIrrelevantScenario() matchInput {
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
	return matchInput{
		Steps:     []realroutes.Step{{Mode: "train"}, {Mode: "plane"}},
		Main:      []int{0, 1},
		Transfers: map[int]time.Duration{},
		Available: [][]observed{trains, flights},
		From:      base,
		End:       base.Add(6 * 24 * time.Hour),
		Config:    cfg,
	}
}

// longDenseChainScenario is a 6-leg scheme (the practical limit before
// schemes.MaxJourney-style caps stop making sense) with a couple dozen
// departures per leg spread over 10 days, independent of one another — the
// shape that makes brute-force combinations grow fastest.
func longDenseChainScenario() matchInput {
	return benchScenario(6, 25, 10*24*time.Hour)
}

// sparseNoMatchScenario has plenty of departures but none of them can ever
// connect: every leg's departures live in a disjoint time band. This is the
// case where pre-filtering has nothing to remove and should not meaningfully
// regress compared to brute force.
func sparseNoMatchScenario() matchInput {
	base := stamp("2027-03-01T00:00:00Z")
	legs := 3
	steps := make([]realroutes.Step, legs)
	main := make([]int, legs)
	available := make([][]observed, legs)
	for i := range legs {
		steps[i] = realroutes.Step{Mode: "plane"}
		main[i] = i
		band := base.Add(time.Duration(i) * 30 * 24 * time.Hour) // months apart: never connectable
		deps := make([]observed, 20)
		for j := range deps {
			start := band.Add(time.Duration(j) * time.Hour)
			deps[j] = synthDeparture(start, start.Add(90*time.Minute))
		}
		available[i] = deps
	}
	cfg := policy()
	cfg.MaxCombinations = 100000
	cfg.MaxJourneys = 5
	return matchInput{Steps: steps, Main: main, Transfers: map[int]time.Duration{}, Available: available, From: base, End: base.Add(365 * 24 * time.Hour), Config: cfg}
}

func BenchmarkMatch_ManyIrrelevant(b *testing.B) {
	benchmarkMatchers(b, manyIrrelevantScenario())
}
func BenchmarkMatch_LongDenseChain(b *testing.B) {
	benchmarkMatchers(b, longDenseChainScenario())
}
func BenchmarkMatch_SparseNoMatches(b *testing.B) {
	benchmarkMatchers(b, sparseNoMatchScenario())
}

func benchmarkMatchers(b *testing.B, in matchInput) {
	b.Run("brute", func(b *testing.B) {
		b.ReportAllocs()
		var attempts int
		b.ResetTimer()
		for range b.N {
			_, _, attempts = searchCounting(context.Background(), in, in.Available)
		}
		b.ReportMetric(float64(attempts), "combinations/op")
	})
	b.Run("windowed", func(b *testing.B) {
		b.ReportAllocs()
		var attempts int
		b.ResetTimer()
		for range b.N {
			filtered := narrowWindows(in)
			_, _, attempts = searchCounting(context.Background(), in, filtered)
		}
		b.ReportMetric(float64(attempts), "combinations/op")
	})
}
