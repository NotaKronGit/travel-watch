package realroutes

import (
	"context"
	"testing"
)

// Synthetic fixture: a city whose rail station has the same provider title.
func sameTitleSteps() []Step {
	return []Step{
		{From: "Курск", To: "Курск", Mode: "transfer", Evidence: "assumed"},
		{From: "Курск", To: "Москва (Киевский вокзал)", FromCode: "s9600816", ToCode: "s2000007", Mode: "train", Evidence: "yandex-rasp"},
		{From: "Москва (Киевский вокзал)", To: "Шереметьево", Mode: "transfer", Evidence: "assumed"},
		{From: "Шереметьево", To: "Суварнабхуми", FromCode: "s9600213", ToCode: "s9623549", Mode: "plane", Evidence: "yandex-rasp"},
		{From: "Суварнабхуми", To: "Паттайя", Mode: "transfer", Evidence: "assumed"},
	}
}
func TestAnnotateTransfersAndLabels(t *testing.T) {
	steps := sameTitleSteps()
	AnnotateTransfers(steps, Query{OriginID: "c1", DestinationID: "c2", OriginName: "Курск", DestinationName: "Паттайя"})
	for _, tc := range []struct {
		i                  int
		from, to           Endpoint
		fromLabel, toLabel string
	}{
		{0, Endpoint{"city", "city:c1", "Курск"}, Endpoint{"station", "s9600816", "Курск"}, "Курск (город)", "Курск (ж/д станция)"},
		{2, Endpoint{"station", "s2000007", "Москва (Киевский вокзал)"}, Endpoint{"airport", "s9600213", "Шереметьево"}, "Москва (Киевский вокзал)", "Шереметьево"},
		{4, Endpoint{"airport", "s9623549", "Суварнабхуми"}, Endpoint{"city", "city:c2", "Паттайя"}, "Суварнабхуми", "Паттайя (город)"},
	} {
		s := steps[tc.i]
		if s.FromPoint == nil || s.ToPoint == nil || *s.FromPoint != tc.from || *s.ToPoint != tc.to {
			t.Fatalf("step %d endpoints %+v %+v", tc.i, s.FromPoint, s.ToPoint)
		}
		if from, to := TransferLabels(s); from != tc.fromLabel || to != tc.toLabel {
			t.Fatalf("step %d labels %q → %q", tc.i, from, to)
		}
	}
	if steps[1].FromPoint != nil || steps[3].ToPoint != nil {
		t.Fatal("transport legs got transfer endpoints")
	}
}
func TestAnnotateTransfersKeepsAndDerivesSafely(t *testing.T) {
	// Saved before endpoints existed: the query has names but no city ids.
	legacy := sameTitleSteps()
	AnnotateTransfers(legacy, Query{OriginName: "Курск", DestinationName: "Паттайя"})
	if p := legacy[0].FromPoint; p == nil || p.Kind != "city" || p.Code != "" {
		t.Fatalf("legacy origin %+v", p)
	}
	// A first transfer that does not start at the requested city is not called a city.
	other := sameTitleSteps()
	AnnotateTransfers(other, Query{OriginName: "Орёл", DestinationName: "Паттайя"})
	if other[0].FromPoint != nil {
		t.Fatal("unrelated origin marked as city")
	}
	if from, to := TransferLabels(other[0]); from != "Курск" || to != "Курск (ж/д станция)" {
		t.Fatalf("labels %q → %q", from, to)
	}
	// Existing endpoints win; airports keep the IATA form of Google Flights legs.
	set := &Endpoint{Kind: "city", Code: "city:kept", Title: "Курск"}
	steps := []Step{
		{From: "Курск", To: "Курск", Mode: "transfer", FromPoint: set},
		{From: "Курск", To: "Пулково (LED)", FromCode: "iata:URS", ToCode: "iata:LED", Mode: "plane"},
	}
	AnnotateTransfers(steps, Query{OriginID: "c1", OriginName: "Курск"})
	if steps[0].FromPoint != set || *steps[0].ToPoint != (Endpoint{"airport", "iata:URS", "Курск"}) {
		t.Fatalf("endpoints %+v %+v", steps[0].FromPoint, steps[0].ToPoint)
	}
	if _, to := TransferLabels(steps[0]); to != "Курск (аэропорт)" {
		t.Fatalf("airport label %q", to)
	}
}
func TestValidEndpointCode(t *testing.T) {
	for code, ok := range map[string]bool{"city:c1": true, "s9600816": true, "iata:SVO": true, "city:": false, "s": false, "s96x": false, "iata:svo": false, "SVO": false, "Курск": false, "": false} {
		if ValidEndpointCode(code) != ok {
			t.Fatalf("%q: want %v", code, ok)
		}
	}
}
func TestPlannerAnnotatesEveryTransfer(t *testing.T) {
	p, q := fixture()
	q.OriginID, q.DestinationID = "o", "d"
	r, err := p.Plan(context.Background(), q)
	if err != nil || len(r.Candidates) == 0 {
		t.Fatal(err)
	}
	for _, c := range r.Candidates {
		for _, steps := range append([][]Step{c.Steps}, c.RailAccessVariants...) {
			for i, s := range steps {
				if s.Mode != "transfer" {
					continue
				}
				if s.FromPoint == nil || s.ToPoint == nil || s.FromPoint.Code == "" || s.ToPoint.Code == "" {
					t.Fatalf("transfer %d without coded endpoints: %+v", i, s)
				}
			}
		}
		if c.Steps[0].FromPoint.Code != "city:o" || c.Steps[len(c.Steps)-1].ToPoint.Code != "city:d" {
			t.Fatalf("request cities missing: %+v", c.Steps)
		}
	}
}
