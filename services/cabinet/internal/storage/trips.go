package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
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
	defer tx.Rollback()
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
	rows, err := tx.QueryContext(ctx, "SELECT id, timezone FROM catalog_cities WHERE id IN ($1,$2) AND active AND name_ru<>'' ORDER BY id", trip.OriginID, trip.DestinationID)
	if err != nil {
		return "", err
	}
	count := 0
	zone := ""
	for rows.Next() {
		var id, tz string
		if err := rows.Scan(&id, &tz); err != nil {
			rows.Close()
			return "", err
		}
		count++
		if id == trip.OriginID {
			zone = tz
		}
	}
	err = rows.Err()
	rows.Close()
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
	q, args, err = postgres.Insert("trip_requests").Rows(goqu.Record{"user_id": trip.UserID, "request_id": trip.RequestID, "origin_id": trip.OriginID, "destination_id": trip.DestinationID, "departure_from": trip.DepartureFrom, "departure_to": trip.DepartureTo, "adults": trip.Adults}).Returning("id").Prepared(true).ToSQL()
	if err != nil {
		return "", err
	}
	var id string
	if err := tx.QueryRowContext(ctx, q, args...).Scan(&id); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

var ErrPastDeparture = errors.New("departure in the past")
