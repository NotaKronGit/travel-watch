package rail

import (
	"math"
	"testing"
)

func TestGeographicQuery(t *testing.T) {
	good := StationQuery{Center: &Coordinates{}, RadiusKM: 20, Limit: 10}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []StationQuery{
		{Center: &Coordinates{Latitude: 91}, RadiusKM: 20, Limit: 10},
		{Center: &Coordinates{Longitude: math.NaN()}, RadiusKM: 20, Limit: 10},
		{Center: &Coordinates{}, RadiusKM: math.Inf(1), Limit: 10},
		{Center: &Coordinates{}, RadiusKM: 20, Limit: 10, Country: "RU"},
		{Center: &Coordinates{}, RadiusKM: 20, Limit: 10, Text: "Москва"},
		{Center: &Coordinates{}, RadiusKM: 0, Limit: 10},
		{Center: &Coordinates{}, RadiusKM: 51, Limit: 10},
		{Text: "Москва", Country: "RU", RadiusKM: 20, Limit: 10},
	} {
		if q.Validate() == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
}
