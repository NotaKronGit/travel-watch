package flights

import (
	"testing"
	"time"
)

func TestQueryAndResults(t *testing.T) {
	q := Query{From: "SVO", To: "BKK", Date: "2027-01-10", Adults: 2, Limit: 5, Nonstop: true}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	r := Result{Provider: "fixture", ObservedAt: time.Now(), Query: q, Options: []Option{{DurationMinutes: 600, Legs: []Leg{{From: "SVO", To: "BKK", Number: "TEST1", DepartureLocal: "2027-01-10T23:00:00", ArrivalLocal: "2027-01-11T13:00:00"}}}}}
	if err := r.Validate(q); err != nil {
		t.Fatal(err)
	}
	r.Options[0].Legs[0].To = "HKT"
	if r.Validate(q) == nil {
		t.Fatal("wrong destination accepted")
	}
	for _, bad := range []Query{
		{From: "svo", To: "BKK", Date: q.Date, Adults: 2, Limit: 5},
		{From: "SVO", To: "SVO", Date: q.Date, Adults: 2, Limit: 5},
		{From: "SVO", To: "BKK", Date: "2027-02-30", Adults: 2, Limit: 5},
		{From: "SVO", To: "BKK", Date: q.Date, Adults: 0, Limit: 5},
	} {
		if bad.Validate() == nil {
			t.Fatal("invalid query accepted")
		}
	}
}
