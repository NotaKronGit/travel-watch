//go:build integration

package auth

import (
	"context"
	"database/sql"
	"testing"

	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/catalog"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
)

type testCitySource struct{ snapshot catalog.Snapshot }

func (s testCitySource) Load(context.Context) (catalog.Snapshot, error) { return s.snapshot, nil }

func testCityCatalog(t *testing.T, ctx context.Context, owner, app *sql.DB) {
	t.Helper()
	snapshot := catalog.Snapshot{Source: "test", Version: "v1", Countries: []catalog.Country{{Code: "RU", Name: "Test country"}}, Cities: []catalog.City{
		{SourceID: 1, Name: "Test city", CountryCode: "RU", Timezone: "Europe/Moscow", Aliases: []string{"Тестовый город"}},
		{SourceID: 2, Name: "Other test city", CountryCode: "RU", Timezone: "Europe/Moscow"},
	}}
	store := storage.New(owner)
	run := func(s catalog.Snapshot) {
		t.Helper()
		if _, err := (catalog.Importer{Source: testCitySource{s}, Repository: store, MinCities: 1}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	run(snapshot)
	var id string
	if err := app.QueryRowContext(ctx, "SELECT id FROM catalog_cities WHERE source_id=1").Scan(&id); err != nil {
		t.Fatal(err)
	}
	snapshot.Cities = snapshot.Cities[:1]
	snapshot.Cities[0].Name = "Renamed test city"
	snapshot.Version = "v2"
	run(snapshot)
	run(snapshot)
	var after, name string
	var active bool
	var count int
	if err := app.QueryRowContext(ctx, "SELECT id,name FROM catalog_cities WHERE source_id=1").Scan(&after, &name); err != nil {
		t.Fatal(err)
	}
	if after != id || name != "Renamed test city" {
		t.Fatal("identity or update lost")
	}
	if err := app.QueryRowContext(ctx, "SELECT active FROM catalog_cities WHERE source_id=2").Scan(&active); err != nil || active {
		t.Fatal("missing city not retained as inactive", err)
	}
	if err := app.QueryRowContext(ctx, "SELECT count(*) FROM catalog_cities").Scan(&count); err != nil || count != 2 {
		t.Fatal("duplicate import", err)
	}
	// Force failure after the countries update, bypassing validation to exercise rollback.
	snapshot.Countries[0].Name = "Must roll back"
	snapshot.Cities[0].CountryCode = "XX"
	if err := store.ReplaceCatalog(ctx, snapshot); err == nil {
		t.Fatal("expected foreign key failure")
	}
	if err := app.QueryRowContext(ctx, "SELECT name FROM catalog_countries WHERE code='RU'").Scan(&name); err != nil || name != "Test country" {
		t.Fatal("partial import committed", err)
	}
	if err := app.QueryRowContext(ctx, "SELECT active FROM catalog_cities WHERE source_id=1").Scan(&active); err != nil || !active {
		t.Fatal("failed import hid working catalog", err)
	}
	snapshot.Cities[0].CountryCode = "RU"
	tx, err := owner.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(742019,1)"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceCatalog(ctx, snapshot); err == nil {
		t.Fatal("concurrent publication allowed")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ExecContext(ctx, "UPDATE catalog_cities SET active=false"); err == nil {
		t.Fatal("web role can change catalog")
	}
}
