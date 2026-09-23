package realroutes

import "strings"

// Endpoint identifies one end of an assumed ground transfer. A station's
// provider title can equal its city's name, so titles alone are ambiguous.
type Endpoint struct {
	Kind  string `json:"kind"`           // city, station or airport
	Code  string `json:"code,omitempty"` // city:<id>, Yandex station code or iata:<IATA>
	Title string `json:"title"`
}

// AnnotateTransfers derives transfer endpoints from the request and the
// neighbouring transport legs, keeping endpoints already set. Saved results
// from before endpoints existed are annotated the same way when read; their
// query has no city id, so the city endpoint stays without a code.
func AnnotateTransfers(steps []Step, q Query) {
	last := len(steps) - 1
	for i := range steps {
		s := &steps[i]
		if s.Mode != "transfer" {
			continue
		}
		if s.FromPoint == nil {
			if i == 0 && q.OriginName != "" && s.From == q.OriginName {
				s.FromPoint = cityEndpoint(q.OriginID, s.From)
			} else if i > 0 {
				s.FromPoint = hubEndpoint(steps[i-1].Mode, steps[i-1].ToCode, s.From)
			}
		}
		if s.ToPoint == nil {
			if i == last && q.DestinationName != "" && s.To == q.DestinationName {
				s.ToPoint = cityEndpoint(q.DestinationID, s.To)
			} else if i < last {
				s.ToPoint = hubEndpoint(steps[i+1].Mode, steps[i+1].FromCode, s.To)
			}
		}
	}
}
func cityEndpoint(id, title string) *Endpoint {
	e := &Endpoint{Kind: "city", Title: title}
	if id != "" {
		e.Code = "city:" + id
	}
	return e
}

// hubEndpoint takes the physical node from the adjacent transport leg.
func hubEndpoint(mode, code, title string) *Endpoint {
	if code == "" {
		return nil
	}
	switch mode {
	case "train":
		return &Endpoint{Kind: "station", Code: code, Title: title}
	case "plane", "flight":
		return &Endpoint{Kind: "airport", Code: code, Title: title}
	}
	return nil
}

// TransferLabels names a transfer's ends for display. Cities are always
// marked; a station or airport gets its type only when both ends share a title.
func TransferLabels(s Step) (string, string) {
	typed := s.From == s.To
	return endpointLabel(s.FromPoint, s.From, typed), endpointLabel(s.ToPoint, s.To, typed)
}
func endpointLabel(e *Endpoint, title string, typed bool) string {
	if e == nil {
		return title
	}
	switch {
	case e.Kind == "city":
		return title + " (город)"
	case typed && e.Kind == "station":
		return title + " (ж/д станция)"
	case typed && e.Kind == "airport":
		return title + " (аэропорт)"
	}
	return title
}

// ValidEndpointCode accepts the code forms AnnotateTransfers produces.
func ValidEndpointCode(code string) bool {
	switch {
	case strings.HasPrefix(code, "city:"):
		return len(code) > len("city:")
	case strings.HasPrefix(code, "iata:"):
		return validIATA(strings.TrimPrefix(code, "iata:"))
	case strings.HasPrefix(code, "s") && len(code) > 1:
		return strings.Trim(code[1:], "0123456789") == ""
	}
	return false
}
