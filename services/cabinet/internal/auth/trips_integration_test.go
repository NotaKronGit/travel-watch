//go:build integration

package auth

import (
	"context"
	"database/sql"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"testing"
	"time"
)

func testTripStorage(t *testing.T, ctx context.Context, owner, app *sql.DB) {
	t.Helper()
	var user string
	if err := owner.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES('trip-fixture@example.com','test-only') RETURNING id").Scan(&user); err != nil {
		t.Fatal(err)
	}
	ids := []string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333"}
	for i, id := range ids {
		_, err := owner.ExecContext(ctx, "INSERT INTO catalog_cities(id,source,source_id,name,name_ru,country_code,region_code,timezone,aliases,latitude,longitude,population,active) VALUES($1,'trip-test',$2,'Fixture','Тестовый город','RU','01','Europe/Moscow','[]',0,0,0,$3)", id, i+1, i < 2)
		if err != nil {
			t.Fatal(err)
		}
	}
	store := storage.New(app)
	cities, err := store.SearchCities(ctx, "Тестовый")
	if err != nil || len(cities) < 2 {
		t.Fatal("search failed", err)
	}
	for _, c := range cities {
		if c.ID == ids[2] {
			t.Fatal("inactive city suggested")
		}
	}
	wild, err := store.SearchCities(ctx, "%%")
	if err != nil || len(wild) != 0 {
		t.Fatal("wildcards not escaped", err)
	}
	date := time.Now().AddDate(0, 0, 5).Format(time.DateOnly)
	trip := storage.Trip{UserID: user, RequestID: "44444444-4444-4444-4444-444444444444", OriginID: ids[0], DestinationID: ids[1], DepartureFrom: date, DepartureTo: date, Adults: 2}
	id, err := store.CreateTrip(ctx, trip)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.CreateTrip(ctx, trip)
	if err != nil || again != id {
		t.Fatal("retry duplicated trip", err)
	}
	trip.Adults = 3
	if _, err := store.CreateTrip(ctx, trip); !errors.Is(err, storage.ErrRequestConflict) {
		t.Fatal("changed retry accepted", err)
	}
	trip.RequestID = "55555555-5555-5555-5555-555555555555"
	trip.DestinationID = ids[2]
	if _, err := store.CreateTrip(ctx, trip); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("inactive destination accepted", err)
	}
	trip.DestinationID = ids[1]
	trip.DepartureFrom = "2020-01-01"
	if _, err := store.CreateTrip(ctx, trip); !errors.Is(err, storage.ErrPastDeparture) {
		t.Fatal("past departure accepted", err)
	}
	var count int
	if err := app.QueryRowContext(ctx, "SELECT count(*) FROM trip_requests WHERE user_id=$1", user).Scan(&count); err != nil || count != 1 {
		t.Fatal("invalid request persisted", err)
	}
}
