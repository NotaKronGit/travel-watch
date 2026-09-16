//go:build integration

package storage

import (
	"context"
	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
	"testing"
	"time"
)

func TestAirportImport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, db := testDB(t, ctx)
	repo := New(db)
	first := time.Now().UTC()
	a := airports.Airport{SourceID: 1, Ident: "TEST1", Name: "Test one", Type: "medium_airport", Country: "RU", IATA: "TST"}
	b := a
	b.SourceID = 2
	b.Ident = "TEST2" // repeated IATA is not a repeated source identity
	s := airports.Snapshot{FetchedAt: first, Airports: []airports.Airport{a, b}}
	if err := repo.PublishAirports(ctx, s, 90); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindAirports(ctx, "TST")
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
	id := got[0].ID
	s.FetchedAt = first.Add(time.Second)
	s.Airports[0].Name = "Updated"
	if err = repo.PublishAirports(ctx, s, 90); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindAirports(ctx, "TST")
	if err != nil || got[0].ID != id || got[0].Name != "Updated" {
		t.Fatal("identity lost", err)
	}
	s.Airports = s.Airports[:1]
	s.FetchedAt = first.Add(2 * time.Second)
	if err = repo.PublishAirports(ctx, s, 90); err == nil {
		t.Fatal("shrinking snapshot accepted")
	}
	if err = repo.PublishAirports(ctx, s, 50); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindAirports(ctx, "TST")
	if err != nil || len(got) != 1 {
		t.Fatal("absent airport not deactivated", err)
	}
	s.FetchedAt = first
	if err = repo.PublishAirports(ctx, s, 50); err == nil {
		t.Fatal("stale snapshot accepted")
	}
	// Failure after upsert and deactivation must roll back all three tables' changes.
	if _, err = owner.ExecContext(ctx, "ALTER TABLE airport_imports ADD CONSTRAINT fail_test CHECK (row_count<2) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	s.FetchedAt = first.Add(3 * time.Second)
	s.Airports = []airports.Airport{a, b}
	if err = repo.PublishAirports(ctx, s, 90); err == nil {
		t.Fatal("expected database failure")
	}
	got, err = repo.FindAirports(ctx, "TST")
	if err != nil || len(got) != 1 || got[0].Name != "Updated" {
		t.Fatal("partial import survived rollback", err)
	}
	if _, err = owner.ExecContext(ctx, "ALTER TABLE airport_imports DROP CONSTRAINT fail_test"); err != nil {
		t.Fatal(err)
	}
	if err = repo.PublishAirports(ctx, s, 90); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindAirports(ctx, "TST")
	if err != nil || len(got) != 2 || got[0].ID != id {
		t.Fatal("reactivation failed", err)
	}
}
