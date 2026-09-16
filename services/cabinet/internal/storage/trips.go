package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/doug-martin/goqu/v9"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CityOption struct{ ID, Name, Country, Region, Timezone, IATACode string }
type Trip struct {
	ID, UserID, RequestID, OriginID, DestinationID, DepartureFrom, DepartureTo string
	Adults                                                                     int32
}

var ErrRequestConflict = errors.New("request id reused with different trip")

func (s *Store) SearchCities(ctx context.Context, prefix string) ([]CityOption, error) {
	pattern := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(prefix)) + "%"
	q, args, err := postgres.From(goqu.T("catalog_cities").As("c")).Join(goqu.T("catalog_countries").As("n"), goqu.On(goqu.I("n.code").Eq(goqu.I("c.country_code")))).Select(goqu.I("c.id"), goqu.I("c.name_ru"), goqu.L("COALESCE(NULLIF(n.name_ru, ''), n.name)"), goqu.I("c.region_name"), goqu.I("c.timezone"), goqu.I("c.iata_code")).Where(goqu.I("c.active").IsTrue(), goqu.I("c.name_ru").Neq(""), goqu.L("lower(c.name_ru) LIKE ?", pattern)).Order(goqu.I("c.population").Desc(), goqu.I("c.id").Asc()).Limit(10).Prepared(true).ToSQL()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CityOption{}
	for rows.Next() {
		var c CityOption
		if err := rows.Scan(&c.ID, &c.Name, &c.Country, &c.Region, &c.Timezone, &c.IATACode); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
func (s *Store) CreateTrip(ctx context.Context, trip Trip) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize retries for the same user and request, including concurrent requests.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", trip.UserID+":"+trip.RequestID); err != nil {
		return "", err
	}
	q, args, err := postgres.From("trip_requests").Select("id", "origin_id", "destination_id", goqu.L("departure_from::text"), goqu.L("departure_to::text"), "adults").Where(goqu.Ex{"user_id": trip.UserID, "request_id": trip.RequestID}).Prepared(true).ToSQL()
	if err != nil {
		return "", err
	}
	var previous Trip
	err = tx.QueryRowContext(ctx, q, args...).Scan(&previous.ID, &previous.OriginID, &previous.DestinationID, &previous.DepartureFrom, &previous.DepartureTo, &previous.Adults)
	if err == nil {
		if previous.OriginID != trip.OriginID || previous.DestinationID != trip.DestinationID || previous.DepartureFrom != trip.DepartureFrom || previous.DepartureTo != trip.DepartureTo || previous.Adults != trip.Adults {
			return "", ErrRequestConflict
		}
		return previous.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	// Read both cities in one snapshot; the application only has SELECT on the catalog.
	rows, err := tx.QueryContext(ctx, "SELECT id, source, source_id, name_ru, country_code, latitude, longitude, timezone, iata_code FROM catalog_cities WHERE id IN ($1,$2) AND active AND name_ru<>'' ORDER BY id", trip.OriginID, trip.DestinationID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	count := 0
	zone := ""
	cities := map[string]*eventsv1.TripCity{}
	for rows.Next() {
		var city eventsv1.TripCity
		if err := rows.Scan(&city.Id, &city.Source, &city.SourceId, &city.Name, &city.CountryCode, &city.Latitude, &city.Longitude, &city.Timezone, &city.IataCode); err != nil {
			return "", err
		}
		count++
		cities[city.Id] = &city
		if city.Id == trip.OriginID {
			zone = city.Timezone
		}
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if count != 2 {
		return "", ErrNotFound
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	if trip.DepartureFrom < time.Now().In(location).Format(time.DateOnly) {
		return "", ErrPastDeparture
	}
	q, args, err = postgres.Insert("trip_requests").Rows(goqu.Record{"user_id": trip.UserID, "request_id": trip.RequestID, "origin_id": trip.OriginID, "destination_id": trip.DestinationID, "departure_from": trip.DepartureFrom, "departure_to": trip.DepartureTo, "adults": trip.Adults}).Returning("id", "created_at", goqu.L("gen_random_uuid()")).Prepared(true).ToSQL()
	if err != nil {
		return "", err
	}
	var id, eventID string
	var created time.Time
	if err := tx.QueryRowContext(ctx, q, args...).Scan(&id, &created, &eventID); err != nil {
		return "", err
	}
	event := &eventsv1.TripRequestCreated{EventId: eventID, SchemaVersion: 1, RequestId: id, OccurredAt: timestamppb.New(created), Origin: cities[trip.OriginID], Destination: cities[trip.DestinationID], DepartureFrom: trip.DepartureFrom, DepartureTo: trip.DepartureTo, Adults: trip.Adults}
	payload, err := proto.Marshal(event)
	if err != nil {
		return "", err
	}
	q, args, err = postgres.Insert("outbox_events").Rows(goqu.Record{"id": eventID, "request_id": id, "event_type": "travelwatch.events.v1.TripRequestCreated", "payload": payload}).Prepared(true).ToSQL()
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

var ErrPastDeparture = errors.New("departure in the past")

type TripDetails struct {
	BuildingStage              string
	History                    []*eventsv1.TripRouteBuildingUpdated
	Status, Comment            string
	CancelledAt                sql.NullTime
	ID                         string
	Origin, Destination        CityOption
	DepartureFrom, DepartureTo string
	Adults                     int32
	CreatedAt                  time.Time
}

func tripReadQuery(user string) *goqu.SelectDataset {
	q := postgres.From(goqu.T("trip_requests").As("t"))
	for _, side := range []struct{ alias, column string }{{"o", "origin_id"}, {"d", "destination_id"}} {
		q = q.Join(goqu.T("catalog_cities").As(side.alias), goqu.On(goqu.I(side.alias+".id").Eq(goqu.I("t."+side.column))))
		q = q.Join(goqu.T("catalog_countries").As(side.alias+"n"), goqu.On(goqu.I(side.alias+"n.code").Eq(goqu.I(side.alias+".country_code"))))
	}
	columns := []interface{}{goqu.I("t.id"), goqu.L("t.departure_from::text"), goqu.L("t.departure_to::text"), goqu.I("t.adults"), goqu.I("t.created_at"), goqu.I("t.status"), goqu.I("t.comment"), goqu.I("t.cancelled_at"), goqu.I("t.building_stage")}
	for _, alias := range []string{"o", "d"} {
		columns = append(columns, goqu.I(alias+".id"), goqu.I(alias+".name_ru"), goqu.L("COALESCE(NULLIF(?, ''), ?)", goqu.I(alias+"n.name_ru"), goqu.I(alias+"n.name")), goqu.I(alias+".region_name"), goqu.I(alias+".timezone"), goqu.I(alias+".iata_code"))
	}
	// Inactive catalog entries remain readable for existing requests.
	return q.Select(columns...).Where(goqu.I("t.user_id").Eq(user))
}
func scanTrip(row interface{ Scan(...any) error }) (TripDetails, error) {
	var t TripDetails
	err := row.Scan(&t.ID, &t.DepartureFrom, &t.DepartureTo, &t.Adults, &t.CreatedAt, &t.Status, &t.Comment, &t.CancelledAt, &t.BuildingStage,
		&t.Origin.ID, &t.Origin.Name, &t.Origin.Country, &t.Origin.Region, &t.Origin.Timezone, &t.Origin.IATACode,
		&t.Destination.ID, &t.Destination.Name, &t.Destination.Country, &t.Destination.Region, &t.Destination.Timezone, &t.Destination.IATACode)
	return t, err
}
func (s *Store) GetTrip(ctx context.Context, user, id string) (TripDetails, error) {
	q, args, err := tripReadQuery(user).Where(goqu.I("t.id").Eq(id)).Prepared(true).ToSQL()
	if err != nil {
		return TripDetails{}, err
	}
	t, err := scanTrip(s.db.QueryRowContext(ctx, q, args...))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	if err == nil {
		t.History, err = s.tripHistory(ctx, id)
	}
	return t, err
}
func (s *Store) ListTrips(ctx context.Context, user string, offset uint) ([]TripDetails, bool, error) {
	q, args, err := tripReadQuery(user).Order(goqu.I("t.created_at").Desc(), goqu.I("t.id").Desc()).Limit(21).Offset(offset).Prepared(true).ToSQL()
	if err != nil {
		return nil, false, err
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := []TripDetails{}
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			return nil, false, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(result) > 20
	if more {
		result = result[:20]
	}
	return result, more, nil
}
