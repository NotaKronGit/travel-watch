package consumer

import (
	"errors"
	"math"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

const Created = "travelwatch.events.v1.TripRequestCreated"
const Cancelled = "travelwatch.events.v1.TripRequestCancelled"
const Expired = "travelwatch.events.v1.TripRequestExpired"

var ErrInvalidEvent = errors.New("invalid or unsupported trip event")

type Event struct {
	ID, RequestID, Type string
	OccurredAt          time.Time
	Payload             []byte
}

// Decode accepts headerless creation events emitted before event_type was added.
func Decode(m kafka.Message) (Event, error) {
	kind, headers := Created, 0
	for _, h := range m.Headers {
		if h.Key == "event_type" {
			kind = string(h.Value)
			headers++
		}
	}
	if headers > 1 || len(m.Value) > 1<<20 {
		return Event{}, ErrInvalidEvent
	}
	e := Event{Type: kind, Payload: m.Value}
	switch kind {
	case Created:
		v := new(eventsv1.TripRequestCreated)
		if proto.Unmarshal(m.Value, v) != nil || v.SchemaVersion != 1 || v.OccurredAt == nil || v.OccurredAt.CheckValid() != nil || !validCity(v.Origin) || !validCity(v.Destination) || v.Origin.Id == v.Destination.Id || v.Adults < 1 || v.Adults > 9 {
			return Event{}, ErrInvalidEvent
		}
		from, err := time.Parse(time.DateOnly, v.DepartureFrom)
		to, err2 := time.Parse(time.DateOnly, v.DepartureTo)
		if err != nil || err2 != nil || to.Before(from) {
			return Event{}, ErrInvalidEvent
		}
		e.ID, e.RequestID, e.OccurredAt = v.EventId, v.RequestId, v.OccurredAt.AsTime()
	case Cancelled:
		v := new(eventsv1.TripRequestCancelled)
		if proto.Unmarshal(m.Value, v) != nil || v.SchemaVersion != 1 || v.OccurredAt == nil || v.OccurredAt.CheckValid() != nil {
			return Event{}, ErrInvalidEvent
		}
		e.ID, e.RequestID, e.OccurredAt = v.EventId, v.RequestId, v.OccurredAt.AsTime()
	case Expired:
		v := new(eventsv1.TripRequestExpired)
		if proto.Unmarshal(m.Value, v) != nil || v.SchemaVersion != 1 || v.OccurredAt == nil || v.OccurredAt.CheckValid() != nil {
			return Event{}, ErrInvalidEvent
		}
		e.ID, e.RequestID, e.OccurredAt = v.EventId, v.RequestId, v.OccurredAt.AsTime()
	default:
		return Event{}, ErrInvalidEvent
	}
	if !validID(e.ID) || !validID(e.RequestID) || string(m.Key) != e.RequestID {
		return Event{}, ErrInvalidEvent
	}
	return e, nil
}

func validID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}
func validCity(c *eventsv1.TripCity) bool {
	if c == nil || !validID(c.Id) || c.Name == "" || c.Source == "" || c.SourceId <= 0 || len(c.CountryCode) != 2 || c.Timezone == "" {
		return false
	}
	if math.IsNaN(c.Latitude) || math.IsNaN(c.Longitude) || math.Abs(c.Latitude) > 90 || math.Abs(c.Longitude) > 180 {
		return false
	}
	_, err := time.LoadLocation(c.Timezone)
	return err == nil
}
