package schedules

import (
	"context"
	"time"

	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// matchInput carries everything a matcher needs to search one already-fetched
// candidate scheme for time-compatible journeys. Main indexes the transport
// (non-transfer) steps; Available holds the fetched departures for each of
// them, in the same order.
type matchInput struct {
	Steps     []realroutes.Step
	Main      []int
	Transfers map[int]time.Duration
	Available [][]observed
	Mismatch  bool // adjacent legs don't provably meet; journeys stay preliminary
	From, End time.Time
	Config    Config
}

// matcher searches an already-fetched candidate scheme for compatible
// journeys. Implementations must agree on the set of journeys found for the
// same input; a faster matcher may only visit fewer combinations to get
// there, never a different or incomplete result.
type matcher interface {
	Match(ctx context.Context, in matchInput) (journeys []*v1.ScheduledJourney, truncated bool)
}

// bruteMatcher visits every fetched departure of every leg in chronological
// order, exactly as the first implementation of this package did. It never
// narrows a leg's candidates ahead of time.
type bruteMatcher struct{}

func (bruteMatcher) Match(ctx context.Context, in matchInput) ([]*v1.ScheduledJourney, bool) {
	journeys, truncated, _ := searchCounting(ctx, in, in.Available)
	return journeys, truncated
}

// windowedMatcher first propagates a backward wave of feasible arrival
// windows from the last leg to the first — a departure survives only if some
// departure of the next leg could still connect to it — then runs the same
// search as bruteMatcher over the narrowed lists. The bound is a coarse
// min/max over the next leg's departures rather than an exact union of
// per-departure windows: cheap, one pass, and provably safe (see
// narrowWindows), at the cost of being less tight when the next leg's
// departures are themselves spread out over many days.
type windowedMatcher struct{}

func (windowedMatcher) Match(ctx context.Context, in matchInput) ([]*v1.ScheduledJourney, bool) {
	journeys, truncated, _ := searchCounting(ctx, in, narrowWindows(in))
	return journeys, truncated
}

// searchCounting is the shared combination walk used by every matcher; only
// the candidate lists passed in differ between them. The returned attempt
// count is not part of the matcher contract — it exists so tests and
// benchmarks can compare how much work each matcher actually did.
func searchCounting(ctx context.Context, in matchInput, available [][]observed) ([]*v1.ScheduledJourney, bool, int) {
	var journeys []*v1.ScheduledJourney
	attempts := 0
	truncated := false
	var walk func(n int, legs []*v1.ScheduledLeg, start, previous time.Time)
	walk = func(n int, legs []*v1.ScheduledLeg, start, previous time.Time) {
		if ctx.Err() != nil {
			return
		}
		if len(journeys) >= in.Config.MaxJourneys {
			truncated = true
			return
		}
		if n == len(in.Main) {
			finish := previous.Add(in.Config.after(in.Steps[in.Main[n-1]].Mode))
			for i := in.Main[n-1] + 1; i < len(in.Steps); i++ {
				finish = finish.Add(in.Transfers[i])
			}
			if finish.Sub(start) > in.Config.MaxJourney {
				return
			}
			journeys = append(journeys, &v1.ScheduledJourney{Legs: append([]*v1.ScheduledLeg(nil), legs...), TimingVerified: !in.Mismatch})
			return
		}
		var transferFrom, transferTo string
		var boarding time.Duration
		if n > 0 {
			transferFrom, transferTo = transferBetween(in, n-1)
			if transferFrom != "" {
				boarding = in.Config.before(in.Steps[in.Main[n]].Mode)
			}
		}
		for _, d := range available[n] {
			attempts++
			if attempts > in.Config.MaxCombinations {
				truncated = true
				return
			}
			var required, gap time.Duration
			nextStart := start
			if n == 0 {
				nextStart = d.Departure.Add(-leg0Offset(in))
				if nextStart.Before(in.From) || !nextStart.Before(in.End) {
					continue
				}
			} else {
				required = connectionRequirement(in, n-1)
				gap = d.Departure.Sub(previous)
				if gap < required || gap > in.Config.MaxConnection {
					continue
				}
			}
			if d.Arrival.Sub(nextStart) > in.Config.MaxJourney {
				continue
			}
			leg := &v1.ScheduledLeg{From: d.From.Title, To: d.To.Title, Mode: d.Mode, Number: d.Number, Departure: d.Departure.Format(time.RFC3339), Arrival: d.Arrival.Format(time.RFC3339), ObservedAt: timestamppb.New(d.at), ConnectionMinutes: int64(gap / time.Minute), RequiredMinutes: int64(required / time.Minute), TransferFrom: transferFrom, TransferTo: transferTo, BoardingMinutes: int64(boarding / time.Minute)}
			walk(n+1, append(legs, leg), nextStart, d.Arrival)
			if truncated {
				return
			}
		}
	}
	walk(0, nil, time.Time{}, time.Time{})
	return journeys, truncated, attempts
}

// leg0Offset is how much earlier than the first leg's departure the traveler
// must start: boarding prep plus any transfers ahead of it in Steps.
func leg0Offset(in matchInput) time.Duration {
	offset := in.Config.before(in.Steps[in.Main[0]].Mode)
	for i := 0; i < in.Main[0]; i++ {
		offset += in.Transfers[i]
	}
	return offset
}

// connectionRequirement is the minimum gap between the arrival of leg k and
// the departure of leg k+1: deboarding, buffer, boarding prep, plus any
// transfers between them in Steps.
func connectionRequirement(in matchInput, k int) time.Duration {
	required := in.Config.after(in.Steps[in.Main[k]].Mode) + in.Config.Buffer + in.Config.before(in.Steps[in.Main[k+1]].Mode)
	for i := in.Main[k] + 1; i < in.Main[k+1]; i++ {
		required += in.Transfers[i]
	}
	return required
}

// narrowWindows propagates a backward wave of feasible arrival windows from
// the last leg to the first, then filters the first leg by the request's own
// departure window. Each step only removes a departure when no departure of
// the (already-narrowed) next leg could possibly connect to it — the coarse
// min/max bound below is a superset of the true union of per-departure
// windows, so this can never discard a journey that bruteMatcher would find.
func narrowWindows(in matchInput) [][]observed {
	n := len(in.Available)
	filtered := make([][]observed, n)
	filtered[n-1] = in.Available[n-1]
	for k := n - 2; k >= 0; k-- {
		required := connectionRequirement(in, k)
		filtered[k] = filterByArrival(in.Available[k], filtered[k+1], in.Config.MaxConnection, required)
	}
	lo, hi := in.From.Add(leg0Offset(in)), in.End.Add(leg0Offset(in))
	filtered[0] = filterByDeparture(filtered[0], lo, hi)
	return filtered
}

// filterByArrival keeps only candidates whose arrival falls within
// [min(next departures)-maxConnection, max(next departures)-required] — a
// safe superset of the union of each next departure's own narrow window,
// since every individual window is contained in this coarser bound.
func filterByArrival(candidates, next []observed, maxConnection, required time.Duration) []observed {
	if len(next) == 0 {
		return nil
	}
	lo, hi := next[0].Departure, next[0].Departure
	for _, d := range next[1:] {
		if d.Departure.Before(lo) {
			lo = d.Departure
		}
		if d.Departure.After(hi) {
			hi = d.Departure
		}
	}
	lo, hi = lo.Add(-maxConnection), hi.Add(-required)
	return filterExact(candidates, func(d observed) bool { return !d.Arrival.Before(lo) && !d.Arrival.After(hi) })
}

// filterByDeparture keeps only candidates whose departure falls in [lo, hi).
func filterByDeparture(candidates []observed, lo, hi time.Time) []observed {
	return filterExact(candidates, func(d observed) bool { return !d.Departure.Before(lo) && d.Departure.Before(hi) })
}

// filterExact makes two passes over candidates so the result is allocated at
// its exact final size — neither over-allocated (wasted zeroing when the
// filter removes most candidates, the common case this file exists for) nor
// grown incrementally one append at a time (extra copies when the filter
// removes almost nothing, which happens whenever MaxConnection is wide
// relative to how tightly the next leg's departures cluster).
func filterExact(candidates []observed, keep func(observed) bool) []observed {
	n := 0
	for _, d := range candidates {
		if keep(d) {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	result := make([]observed, 0, n)
	for _, d := range candidates {
		if keep(d) {
			result = append(result, d)
		}
	}
	return result
}

// transferBetween names the ground transfer between main legs k and k+1 as the
// first transfer's origin and the last one's destination; empty if there is none.
func transferBetween(in matchInput, k int) (string, string) {
	first, last := -1, -1
	for i := in.Main[k] + 1; i < in.Main[k+1]; i++ {
		if in.Steps[i].Mode == "transfer" {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return "", ""
	}
	from, _ := realroutes.TransferLabels(in.Steps[first])
	_, to := realroutes.TransferLabels(in.Steps[last])
	return from, to
}
