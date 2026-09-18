package storage

import (
	"encoding/json"
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
			groups[i].count++
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
		groups = append(groups, group{candidate: c, count: 1})
	}
	result := make([]realroutes.Candidate, 0, len(groups))
	for _, g := range groups {
		c := g.candidate
		if g.count > 1 {
			steps := c.Steps
			flight := steps[3]
			flight.Number = "" // Specific services are selected at the timetable stage.
			c.Steps = []realroutes.Step{
				{From: steps[0].From, To: flight.From + " (поезд до вокзала и переезд в аэропорт)", Mode: "train", Evidence: "rail-access"},
				flight, steps[4],
			}
			c.Warnings = append(c.Warnings, fmt.Sprintf("Объединено вариантов подвоза поездом: %d. Конкретные поезда, вокзалы и время переезда будут проверены на этапе стыковок.", g.count))
		}
		result = append(result, c)
	}
	return result
}

func railAccessKey(c realroutes.Candidate) string {
	s := c.Steps
	if len(s) != 5 || s[0].Mode != "transfer" || s[1].Mode != "train" || s[2].Mode != "transfer" || (s[3].Mode != "plane" && s[3].Mode != "flight") || s[4].Mode != "transfer" {
		return ""
	}
	if s[0].From == "" || s[1].FromCode == "" || s[3].FromCode == "" || s[3].ToCode == "" || s[4].To == "" {
		return ""
	}
	// Require a continuous original scheme before abstracting station details.
	if s[0].To != s[1].From || s[1].To != s[2].From || s[2].To != s[3].From || s[3].To != s[4].From {
		return ""
	}
	key, _ := json.Marshal([]string{s[0].From, s[1].FromCode, s[3].FromCode, s[3].ToCode, s[4].To, s[0].Evidence, s[1].Evidence, s[2].Evidence, s[3].Evidence, s[4].Evidence})
	return string(key)
}
