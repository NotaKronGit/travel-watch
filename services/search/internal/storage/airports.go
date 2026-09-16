package storage

import (
	"context"
	"database/sql"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
	"github.com/doug-martin/goqu/v9"
	"time"
)

func (s *Store) PublishAirports(ctx context.Context, snapshot airports.Snapshot, minRetained int) error {
	if minRetained < 1 || minRetained > 100 {
		return errors.New("invalid airport retention threshold")
	}
	if err := airports.Validate(ctx, snapshot, 1, 200000); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize publication and reject a download older than the published snapshot.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(71429301)"); err != nil {
		return err
	}
	var previous int
	var fetched time.Time
	err = tx.QueryRowContext(ctx, "SELECT row_count,fetched_at FROM airport_imports WHERE source='ourairports'").Scan(&previous, &fetched)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if !fetched.IsZero() && (!snapshot.FetchedAt.After(fetched) || int64(len(snapshot.Airports))*100 < int64(previous)*int64(minRetained)) {
		return errors.New("airport snapshot is stale or has shrunk beyond configured threshold")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE catalog_airports SET active=false WHERE active"); err != nil {
		return err
	}
	for start := 0; start < len(snapshot.Airports); start += 500 {
		end := min(start+500, len(snapshot.Airports))
		records := make([]goqu.Record, 0, end-start)
		for _, a := range snapshot.Airports[start:end] {
			records = append(records, goqu.Record{"source_id": a.SourceID, "ident": a.Ident, "name": a.Name, "type": a.Type, "latitude": a.Latitude, "longitude": a.Longitude, "country": a.Country, "region": a.Region, "municipality": a.Municipality, "iata": a.IATA, "icao": a.ICAO, "scheduled": a.Scheduled, "active": true, "imported_at": goqu.L("now()")})
		}
		q, args, e := postgres.Insert("catalog_airports").Rows(records).OnConflict(goqu.DoUpdate("source_id", goqu.Record{
			"ident": goqu.L("EXCLUDED.ident"), "name": goqu.L("EXCLUDED.name"), "type": goqu.L("EXCLUDED.type"), "latitude": goqu.L("EXCLUDED.latitude"), "longitude": goqu.L("EXCLUDED.longitude"), "country": goqu.L("EXCLUDED.country"), "region": goqu.L("EXCLUDED.region"), "municipality": goqu.L("EXCLUDED.municipality"), "iata": goqu.L("EXCLUDED.iata"), "icao": goqu.L("EXCLUDED.icao"), "scheduled": goqu.L("EXCLUDED.scheduled"), "active": true, "imported_at": goqu.L("EXCLUDED.imported_at"),
		})).Prepared(true).ToSQL()
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, q, args...); e != nil {
			return e
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO airport_imports(source,row_count,fetched_at,imported_at) VALUES('ourairports',$1,$2,now()) ON CONFLICT(source) DO UPDATE SET row_count=EXCLUDED.row_count,fetched_at=EXCLUDED.fetched_at,imported_at=EXCLUDED.imported_at`, len(snapshot.Airports), snapshot.FetchedAt); err != nil {
		return err
	}
	return tx.Commit()
}

type AirportRecord struct {
	ID string `json:"id"`
	airports.Airport
	ImportedAt time.Time `json:"imported_at"`
}

func (s *Store) FindAirports(ctx context.Context, iata string) ([]AirportRecord, error) {
	q, args, err := postgres.From("catalog_airports").Select("id", "source_id", "ident", "name", "type", "latitude", "longitude", "country", "region", "municipality", "iata", "icao", "scheduled", "imported_at").Where(goqu.Ex{"iata": iata, "active": true}).Order(goqu.C("source_id").Asc()).Limit(100).Prepared(true).ToSQL()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AirportRecord{}
	for rows.Next() {
		var a AirportRecord
		if err = rows.Scan(&a.ID, &a.SourceID, &a.Ident, &a.Name, &a.Type, &a.Latitude, &a.Longitude, &a.Country, &a.Region, &a.Municipality, &a.IATA, &a.ICAO, &a.Scheduled, &a.ImportedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
