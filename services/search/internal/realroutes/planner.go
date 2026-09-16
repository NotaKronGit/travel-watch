package realroutes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
)

type run struct {
	p        Planner
	r        Result
	cache    map[string]transport.Response
	failures map[string]bool
	seen     map[string]bool
}

func (p Planner) Plan(ctx context.Context, q Query) (Result, error) {
	if err := p.Config.Validate(); err != nil {
		return Result{}, err
	}
	if p.Provider == nil || q.OriginName == "" || q.DestinationName == "" {
		return Result{}, errors.New("invalid route query")
	}
	for _, pt := range []transport.Point{q.Origin, q.Destination} {
		if math.IsNaN(pt.Latitude) || math.IsNaN(pt.Longitude) || math.Abs(pt.Latitude) > 90 || math.Abs(pt.Longitude) > 180 {
			return Result{}, errors.New("invalid route coordinates")
		}
	}
	s := run{p: p, r: Result{Query: q, ObservedAt: time.Now().UTC(), Source: "Yandex Rasp + OurAirports", Issues: []string{}, Candidates: []Candidate{}}, cache: map[string]transport.Response{}, failures: map[string]bool{}, seen: map[string]bool{}}
	origin := s.near(q.Origin, p.Config.AirportRadiusKM)
	dest := s.near(q.Destination, p.Config.AirportRadiusKM)
	if len(dest) == 0 {
		s.issue("No destination airports in catalog/radius")
	}
	city, ok := s.call(ctx, transport.Request{Method: "settlement", Center: &q.Origin, Radius: 20})
	cityCode := ""
	if ok && len(city.Stations) == 1 {
		cityCode = city.Stations[0].Code
	}
	// Direct routes are checked independently of incoming-flight discovery order.
	for _, a := range origin {
		for _, b := range dest {
			if a.IATA == b.IATA {
				continue
			}
			for _, leg := range s.search(ctx, a.IATA, b.IATA, "iata", "plane") {
				s.add([]Step{transfer(q.OriginName, leg.From.Title), observed(leg), transfer(leg.To.Title, q.DestinationName)})
			}
		}
	}
	byIATA := map[string][]airports.Airport{}
	for _, a := range p.Airports {
		if a.IATA != "" {
			byIATA[a.IATA] = append(byIATA[a.IATA], a)
		}
	}
	incoming := []transport.Connection{}
	// Discover departure hubs independently from destination schedule sampling.
	geographic := []airports.Airport{}
	for _, a := range p.Airports {
		if a.Scheduled && a.Type == "large_airport" && a.IATA != "" && len(byIATA[a.IATA]) == 1 && distance(q.Origin, point(a)) <= p.Config.HubRadiusKM {
			geographic = append(geographic, a)
		}
	}
	sort.SliceStable(geographic, func(i, j int) bool {
		return distance(q.Origin, point(geographic[i])) < distance(q.Origin, point(geographic[j]))
	})
	if len(geographic) > p.Config.MaxHubs {
		geographic = geographic[:p.Config.MaxHubs]
		s.r.LimitReached = true
	}
	for _, a := range geographic {
		for _, b := range dest {
			if a.IATA == b.IATA {
				continue
			}
			for _, leg := range s.search(ctx, a.IATA, b.IATA, "iata", "plane") {
				// search responses may omit IATA; the provider query fixes these endpoints.
				leg.From.IATA, leg.To.IATA = a.IATA, b.IATA
				incoming = append(incoming, leg)
			}
		}
	}
	for _, arrival := range dest {
		discoveryCalls := 0
		discoveryBudget := max(1, p.Config.MaxRequests/2/max(1, len(dest)))
		uids := map[string]bool{}
		for page := 0; page < p.Config.MaxPages; page++ {
			r, ok := s.call(ctx, transport.Request{Method: "arrivals", Code: arrival.IATA, Offset: page * 100})
			if !ok {
				break
			}
			for _, thread := range r.Threads {
				sampleKey := thread.Number
				if sampleKey == "" {
					sampleKey = thread.UID
				}
				if uids[sampleKey] {
					continue
				}
				uids[sampleKey] = true
				// Reserve half the request budget for access and feeder queries.
				if discoveryCalls >= discoveryBudget {
					s.r.LimitReached = true
					break
				}
				discoveryCalls++
				t, ok := s.call(ctx, transport.Request{Method: "thread", UID: thread.UID})
				if !ok {
					continue
				}
				for _, leg := range t.Connections {
					if leg.To.IATA == arrival.IATA && leg.From.IATA != "" {
						incoming = append(incoming, leg)
					} else {
						s.issue("Flight endpoint mapping missing or mismatched")
					}
				}
			}
			if page*100+r.Count >= r.Total {
				break
			}
			if page+1 == p.Config.MaxPages {
				s.r.LimitReached = true
			}
			if discoveryCalls >= discoveryBudget {
				break
			}
		}
	}
	sort.SliceStable(incoming, func(i, j int) bool {
		aa, bb := byIATA[incoming[i].From.IATA], byIATA[incoming[j].From.IATA]
		if len(aa) != 1 {
			return false
		}
		if len(bb) != 1 {
			return true
		}
		return distance(q.Origin, point(aa[0])) < distance(q.Origin, point(bb[0]))
	})
	hubs := map[string]bool{}
	pairs := map[string]bool{}
	for _, flight := range incoming {
		pair := flight.From.Code + "/" + flight.To.Code
		if pairs[pair] {
			continue
		}
		pairs[pair] = true
		if !hubs[flight.From.Code] && len(hubs) >= p.Config.MaxHubs {
			s.r.LimitReached = true
			continue
		}
		hubs[flight.From.Code] = true
		matches := byIATA[flight.From.IATA]
		if len(matches) != 1 {
			s.issue("Hub airport has no unambiguous catalog mapping")
			continue
		}
		hub := matches[0]
		if distance(q.Origin, point(hub)) <= p.Config.AirportRadiusKM {
			s.add([]Step{transfer(q.OriginName, flight.From.Title), observed(flight), transfer(flight.To.Title, q.DestinationName)})
			continue
		}
		if cityCode != "" {
			hp := point(hub)
			stations, ok := s.call(ctx, transport.Request{Method: "stations", Center: &hp, Radius: p.Config.RailRadiusKM})
			if ok {
				for _, station := range stations.Stations {
					for _, train := range s.search(ctx, cityCode, station.Code, "yandex", "train") {
						s.add([]Step{transfer(q.OriginName, train.From.Title), observed(train), transfer(train.To.Title, flight.From.Title), observed(flight), transfer(flight.To.Title, q.DestinationName)})
					}
				}
			}
		}
		// Local and international flights may use distinct nearby airports.
		for _, feederArrival := range s.near(point(hub), p.Config.RailRadiusKM) {
			for _, a := range origin {
				if a.IATA == feederArrival.IATA {
					continue
				}
				for _, local := range s.search(ctx, a.IATA, feederArrival.IATA, "iata", "plane") {
					if local.From.Code == flight.To.Code {
						continue
					}
					steps := []Step{transfer(q.OriginName, local.From.Title), observed(local)}
					if local.To.Code != flight.From.Code {
						steps = append(steps, transfer(local.To.Title, flight.From.Title))
					}
					steps = append(steps, observed(flight), transfer(flight.To.Title, q.DestinationName))
					s.add(steps)
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return s.r, err
	}
	s.issue("Provider coverage and timetable compatibility are not complete; transfers are assumptions")
	return s.r, nil
}
func (s *run) issue(v string) {
	for _, old := range s.r.Issues {
		if old == v {
			return
		}
	}
	s.r.Issues = append(s.r.Issues, v)
}
func (s *run) call(ctx context.Context, q transport.Request) (transport.Response, bool) {
	b, _ := json.Marshal(q)
	key := string(b)
	if r, ok := s.cache[key]; ok {
		return r, true
	}
	if s.failures[key] {
		return transport.Response{}, false
	}
	if ctx.Err() != nil {
		return transport.Response{}, false
	}
	if s.r.Requests >= s.p.Config.MaxRequests {
		s.r.LimitReached = true
		return transport.Response{}, false
	}
	s.r.Requests++
	r, err := s.p.Provider.Call(ctx, q)
	if err != nil {
		s.r.ProviderFailures++
		s.failures[key] = true
		s.issue(fmt.Sprintf("Collector %s failed", q.Method))
		return r, false
	}
	if r.Incomplete {
		s.issue("Incomplete provider page: " + q.Method)
	}
	s.cache[key] = r
	return r, true
}
func (s *run) search(ctx context.Context, from, to, system, mode string) []transport.Connection {
	result := []transport.Connection{}
	seen := map[string]bool{}
	for page := 0; page < s.p.Config.MaxPages; page++ {
		r, ok := s.call(ctx, transport.Request{Method: "search", From: from, To: to, System: system, Mode: mode, Offset: page * 100})
		if !ok {
			break
		}
		for _, c := range r.Connections {
			key := c.From.Code + "/" + c.To.Code
			if !seen[key] {
				seen[key] = true
				result = append(result, c)
			}
		}
		if page*100+r.Count >= r.Total {
			break
		}
		if r.Count == 0 {
			s.issue("Empty page before provider total")
			break
		}
		if page+1 == s.p.Config.MaxPages {
			s.r.LimitReached = true
		}
	}
	return result
}
func (s *run) near(pt transport.Point, radius float64) []airports.Airport {
	a := []airports.Airport{}
	counts := map[string]int{}
	for _, v := range s.p.Airports {
		counts[v.IATA]++
	}
	for _, v := range s.p.Airports {
		if v.IATA != "" && counts[v.IATA] > 1 {
			s.issue("Ambiguous IATA excluded")
			continue
		}
		if v.IATA != "" && v.Scheduled && (v.Type == "large_airport" || v.Type == "medium_airport") && distance(pt, point(v)) <= radius {
			a = append(a, v)
		}
	}
	sort.Slice(a, func(i, j int) bool {
		di, dj := distance(pt, point(a[i])), distance(pt, point(a[j]))
		if di == dj {
			return a[i].SourceID < a[j].SourceID
		}
		return di < dj
	})
	if len(a) > s.p.Config.MaxAirports {
		s.r.LimitReached = true
		a = a[:s.p.Config.MaxAirports]
	}
	return a
}
func transfer(a, b string) Step { return Step{From: a, To: b, Mode: "transfer", Evidence: "assumed"} }
func observed(c transport.Connection) Step {
	return Step{From: c.From.Title, To: c.To.Title, FromCode: c.From.Code, ToCode: c.To.Code, Mode: c.Mode, Evidence: "yandex-rasp", Number: c.Number}
}
func (s *run) add(steps []Step) {
	// Dedupe topology, not flight/train numbers or schedule variants.
	key := ""
	for _, v := range steps {
		key += v.FromCode + "/" + v.ToCode + "\x00" + v.From + "\x00" + v.To + "\x00" + v.Mode + "\x00"
	}
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	if len(s.r.Candidates) >= s.p.Config.MaxCandidates {
		s.r.LimitReached = true
		return
	}
	s.r.Candidates = append(s.r.Candidates, Candidate{Steps: steps, Warnings: []string{"Transfers are assumed; availability and connection times are not verified"}})
}
