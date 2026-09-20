package realroutes

import (
	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
	"sort"
	"strings"
)

// Round-robin municipalities before a second airport in the same municipality.
// Missing municipality never groups unrelated airports. This is catalog metadata,
// not a claim that municipal boundaries describe every metropolitan airport group.
func diverseAirports(input []airports.Airport, origin transport.Point) []airports.Airport {
	sorted := append([]airports.Airport(nil), input...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type == "large_airport"
		}
		a, b := distance(origin, point(sorted[i])), distance(origin, point(sorted[j]))
		if a == b {
			return sorted[i].IATA < sorted[j].IATA
		}
		return a < b
	})
	groups := [][]airports.Airport{}
	index := map[string]int{}
	seen := map[string]bool{}
	for _, a := range sorted {
		if a.IATA == "" || seen[a.IATA] {
			continue
		}
		seen[a.IATA] = true
		municipality := strings.ToLower(strings.TrimSpace(a.Municipality))
		key := a.Country + "/" + a.Region + "/" + municipality
		if municipality == "" {
			key = "iata:" + a.IATA
		}
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], a)
	}
	result := []airports.Airport{}
	for round := 0; ; round++ {
		added := false
		for _, group := range groups {
			if round < len(group) {
				result = append(result, group[round])
				added = true
			}
		}
		if !added {
			return result
		}
	}
}
