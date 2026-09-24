package rail

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var (
	// ErrTrainNotFound: the provider does not know this train on this date between these stations.
	ErrTrainNotFound = errors.New("railway train not found")
	// ErrNotOnSale: the train is known, but seats are not sold now (sales not open yet or already closed).
	ErrNotOnSale = errors.New("railway seats not on sale")
)

// CarClass is the normalized class of a car; providers keep their own label in ServiceClass.
type CarClass string

const (
	ClassPlatzkart CarClass = "platzkart"
	ClassCoupe     CarClass = "coupe"
	ClassSV        CarClass = "sv"
	ClassLux       CarClass = "lux"
	ClassSeated    CarClass = "seated"
	ClassCommon    CarClass = "common"
)

// SeatPosition is known from the provider or from standard numbering of coupe and platzkart cars.
// Seated and common cars have no tiers: PositionSeat. PositionUnknown is not "upper".
type SeatPosition string

const (
	PositionLower     SeatPosition = "lower"
	PositionUpper     SeatPosition = "upper"
	PositionSideLower SeatPosition = "side_lower"
	PositionSideUpper SeatPosition = "side_upper"
	PositionSeat      SeatPosition = "seat"
	PositionUnknown   SeatPosition = "unknown"
)

// SeatQuery selects one train run between two stations of the same provider.
type SeatQuery struct {
	From, To      Ref
	TrainNumber   string
	DepartureDate string // YYYY-MM-DD, local calendar date at the origin station.
}

// Seat is a free seat. Price is for one adult on this seat; nil means unknown, not free.
type Seat struct {
	Number   string // As given by the provider; preserve leading zeroes.
	Position SeatPosition
	Price    *Money
}

// Car lists its free seats only. An empty Seats slice is a car the provider reported
// with no free seats; a car or class missing from the result says nothing about the train.
type Car struct {
	Number       string
	Class        CarClass
	ServiceClass string // Provider label, e.g. "2Э"; optional.
	Seats        []Seat
}

// SeatResult describes one train run at ObservedAt. Complete=false means some cars or
// seats were not returned; a complete result with no free seats means "no seats".
// An error makes the result unusable.
type SeatResult struct {
	Provider           string
	Synthetic          bool
	ObservedAt         time.Time
	Complete           bool
	TrainNumber        string
	Departure, Arrival time.Time
	Cars               []Car
}

type SeatProvider interface {
	FindSeats(context.Context, SeatQuery) (SeatResult, error)
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func (q SeatQuery) Validate() error {
	if q.From.Provider == "" || q.From.Provider != q.To.Provider || q.From.Code == "" || q.To.Code == "" || q.From == q.To || len(q.From.Provider) > 100 || len(q.From.Code) > 200 || len(q.To.Code) > 200 || q.TrainNumber == "" || len(q.TrainNumber) > 20 {
		return ErrInvalidQuery
	}
	date, err := time.Parse(time.DateOnly, q.DepartureDate)
	if err != nil || date.Format(time.DateOnly) != q.DepartureDate {
		return ErrInvalidQuery
	}
	return nil
}

var errInvalidSeats = errors.New("invalid railway seat result")

// Validate checks a normalized provider result before it is returned; adapters must call it.
func (r SeatResult) Validate(q SeatQuery) error {
	if r.Provider != q.From.Provider || r.ObservedAt.IsZero() || r.TrainNumber == "" || r.Departure.IsZero() || !r.Arrival.After(r.Departure) || r.Cars == nil {
		return errInvalidSeats
	}
	cars := map[string]bool{}
	for _, c := range r.Cars {
		if c.Number == "" || cars[c.Number] || !c.Class.valid() || c.Seats == nil {
			return errInvalidSeats
		}
		cars[c.Number] = true
		seats := map[string]bool{}
		for _, s := range c.Seats {
			if s.Number == "" || seats[s.Number] || !s.Position.validFor(c.Class) {
				return errInvalidSeats
			}
			seats[s.Number] = true
			if s.Price != nil && (s.Price.MinorUnits < 0 || !currencyCode.MatchString(s.Price.Currency)) {
				return errInvalidSeats
			}
		}
	}
	return nil
}

func (c CarClass) valid() bool {
	switch c {
	case ClassPlatzkart, ClassCoupe, ClassSV, ClassLux, ClassSeated, ClassCommon:
		return true
	}
	return false
}

// validFor rejects tiers that cannot exist in the class: side seats outside platzkart,
// tiers in seated cars. Unknown is always allowed.
func (p SeatPosition) validFor(c CarClass) bool {
	switch p {
	case PositionUnknown:
		return true
	case PositionSeat:
		return c == ClassSeated || c == ClassCommon
	case PositionSideLower, PositionSideUpper:
		return c == ClassPlatzkart
	case PositionLower, PositionUpper:
		return c == ClassPlatzkart || c == ClassCoupe || c == ClassSV || c == ClassLux
	}
	return false
}
