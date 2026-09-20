package storage

import (
	"fmt"

	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
)

// groupRailAccess is a read projection, never a mutation of saved planner results.
// Only the known rail-access shape is grouped. Flight endpoints must have stable
// provider codes; ambiguous and other shapes remain separate.
func groupRailAccess(candidates []realroutes.Candidate) []realroutes.Candidate {
	type group struct {
		candidate realroutes.Candidate
		count     int
	}
	groups := []group{}
	seen := map[string]int{}
	for _, c := range candidates {
		key := railAccessKey(c)
		if i, ok := seen[key]; key != "" && ok {
			groups[i].count += max(1, len(c.RailAccessVariants))
			for _, warning := range c.Warnings {
				found := false
				for _, old := range groups[i].candidate.Warnings {
					if old == warning {
						found = true
						break
					}
				}
				if !found {
					groups[i].candidate.Warnings = append(groups[i].candidate.Warnings, warning)
				}
			}
			continue
		}
		if key != "" {
			seen[key] = len(groups)
		}
		c.Steps = append([]realroutes.Step(nil), c.Steps...)
		c.Warnings = append([]string(nil), c.Warnings...)
		groups = append(groups, group{candidate: c, count: max(1, len(c.RailAccessVariants))})
	}
	result := make([]realroutes.Candidate, 0, len(groups))
	for _, g := range groups {
		c := g.candidate
		if g.count > 1 && railAccessKey(c) != "" {
			steps := c.Steps
			tail := append([]realroutes.Step(nil), steps[3:]...)
			for i := range tail {
				tail[i].Number = ""
			}
			c.Steps = append([]realroutes.Step{{From: steps[0].From, To: steps[3].From + " (поезд до вокзала и переезд в аэропорт)", Mode: "train", Evidence: "rail-access"}}, tail...)

			c.Warnings = append(c.Warnings, fmt.Sprintf("Объединено вариантов подвоза поездом: %d. Конкретные поезда, вокзалы и время переезда будут проверены на этапе стыковок.", g.count))
		}
		result = append(result, c)
	}
	return result
}

func railAccessKey(c realroutes.Candidate) string { return realroutes.RailAccessKey(c) }
