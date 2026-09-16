package rail

import (
	"regexp"
	"strings"
	"time"
)

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

func (q StationQuery) Validate() error {
	if strings.TrimSpace(q.Text) == "" || len(q.Text) > 200 || !countryCode.MatchString(q.Country) || q.Limit < 1 || q.Limit > 100 {
		return ErrInvalidQuery
	}
	return nil
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
