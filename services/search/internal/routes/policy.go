package routes

// Patterns include city access and egress. The MVP permits at most two main legs.
func patterns() [][]Mode {
	return [][]Mode{
		{Transfer, Flight, Transfer},
		{Transfer, Train, Transfer, Flight, Transfer},
		{Transfer, Flight, Flight, Transfer},
		{Transfer, Flight, Transfer, Flight, Transfer},
	}
}

func validShape(legs []Link, q Query, nodes map[string]Node) bool {
	for _, pattern := range patterns() {
		if len(legs) != len(pattern) {
			continue
		}
		matches := true
		for i, mode := range pattern {
			if legs[i].Mode != mode {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		// A connecting hub must not return to either endpoint city.
		if len(legs) > 3 {
			hub := nodes[legs[1].To].CityID
			if hub == q.OriginCityID || hub == q.DestinationCityID {
				return false
			}
		}
		return true
	}
	return false
}
