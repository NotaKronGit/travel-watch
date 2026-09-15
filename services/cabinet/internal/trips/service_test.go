package trips

import (
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	"testing"
)

func TestValidate(t *testing.T) {
	good := func() *v1.CreateTripRequest {
		return &v1.CreateTripRequest{RequestId: "00000000-0000-0000-0000-000000000001", OriginId: "00000000-0000-0000-0000-000000000002", DestinationId: "00000000-0000-0000-0000-000000000003", DepartureFrom: "2027-01-01", DepartureTo: "2027-01-01", Adults: 1}
	}
	if err := validate(good()); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*v1.CreateTripRequest){func(r *v1.CreateTripRequest) { r.Adults = 0 }, func(r *v1.CreateTripRequest) { r.Adults = 10 }, func(r *v1.CreateTripRequest) { r.DestinationId = r.OriginId }, func(r *v1.CreateTripRequest) { r.OriginId = "city text" }, func(r *v1.CreateTripRequest) { r.DepartureFrom = "2027-02-30" }, func(r *v1.CreateTripRequest) { r.DepartureTo = "2026-01-01" }} {
		r := good()
		change(r)
		if validate(r) == nil {
			t.Fatal("invalid request accepted")
		}
	}
}
