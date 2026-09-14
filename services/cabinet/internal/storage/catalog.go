package storage

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/catalog"
	"github.com/doug-martin/goqu/v9"
)

func (s *Store) ReplaceCatalog(ctx context.Context, snapshot catalog.Snapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked bool
	// Transaction-scoped lock serializes catalog publications across processes.
	if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock(742019, 1)").Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return errors.New("another catalog publication is running")
	}
	exec := func(ds *goqu.InsertDataset) error {
		query, args, err := ds.Prepared(true).ToSQL()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}
	for _, c := range snapshot.Countries {
		if err := exec(postgres.Insert("catalog_countries").Rows(goqu.Record{"code": c.Code, "name": c.Name}).OnConflict(goqu.DoUpdate("code", goqu.Record{"name": goqu.I("excluded.name")}))); err != nil {
			return err
		}
	}
	query, args, err := postgres.Update("catalog_cities").Set(goqu.Record{"active": false}).Where(goqu.Ex{"source": snapshot.Source}).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	for start := 0; start < len(snapshot.Cities); start += 500 {
		rows := make([]goqu.Record, 0, 500)
		for _, c := range snapshot.Cities[start:min(start+500, len(snapshot.Cities))] {
			aliases, err := json.Marshal(c.Aliases)
			if err != nil {
				return err
			}
			rows = append(rows, goqu.Record{"source": snapshot.Source, "source_id": c.SourceID, "name": c.Name, "country_code": c.CountryCode, "region_code": c.RegionCode, "timezone": c.Timezone, "aliases": string(aliases), "latitude": c.Latitude, "longitude": c.Longitude, "population": c.Population, "active": true})
		}
		update := goqu.Record{}
		for _, col := range []string{"name", "country_code", "region_code", "timezone", "aliases", "latitude", "longitude", "population", "active"} {
			update[col] = goqu.I("excluded." + col)
		}
		if err := exec(postgres.Insert("catalog_cities").Rows(rows).OnConflict(goqu.DoUpdate("source, source_id", update))); err != nil {
			return err
		}
	}
	if err := exec(postgres.Insert("catalog_imports").Rows(goqu.Record{"source": snapshot.Source, "version": snapshot.Version, "city_count": len(snapshot.Cities)}).OnConflict(goqu.DoUpdate("source", goqu.Record{"version": snapshot.Version, "city_count": len(snapshot.Cities), "imported_at": goqu.L("now()")}))); err != nil {
		return err
	}
	return tx.Commit()
}
