package rail

import (
	"math"
	"regexp"
	"strings"
	"time"
)

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

func (q StationQuery) Validate() error {
	if q.Center != nil {
		if q.Text != "" || q.Country != "" || q.Limit < 1 || q.Limit > 100 || !q.Center.Valid() || math.IsNaN(q.RadiusKM) || math.IsInf(q.RadiusKM, 0) || q.RadiusKM <= 0 || q.RadiusKM > 50 {
			return ErrInvalidQuery
		}
		return nil
	}
	if q.RadiusKM != 0 {
		return ErrInvalidQuery
	}
	if strings.TrimSpace(q.Text) == "" || len(q.Text) > 200 || !countryCode.MatchString(q.Country) || q.Limit < 1 || q.Limit > 100 {
		return ErrInvalidQuery
	}
	return nil
}

// Valid accepts finite WGS84 coordinates, including (0,0).
func (c Coordinates) Valid() bool {
	return !math.IsNaN(c.Latitude) && !math.IsNaN(c.Longitude) && !math.IsInf(c.Latitude, 0) && !math.IsInf(c.Longitude, 0) && math.Abs(c.Latitude) <= 90 && math.Abs(c.Longitude) <= 180
}
func (q TrainQuery) Validate() error {
	if q.From.Provider == "" || q.From.Provider != q.To.Provider || q.From.Code == "" || q.To.Code == "" || q.From == q.To || len(q.From.Provider) > 100 || len(q.From.Code) > 200 || len(q.To.Code) > 200 || q.Adults < 1 || q.Adults > 9 || q.Limit < 1 || q.Limit > 100 {
		return ErrInvalidQuery
	}
	date, err := time.Parse(time.DateOnly, q.DepartureDate)
	if err != nil || date.Format(time.DateOnly) != q.DepartureDate {
		return ErrInvalidQuery
	}
	return nil
}
