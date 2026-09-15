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

	got, err := store.GetTrip(ctx, user, id)
	if err != nil || got.ID != id || got.Adults != 2 || got.Origin.ID != ids[0] {
		t.Fatal("cannot read own trip", err)
	}
	var otherUser string
	if err := owner.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES('other-trip-fixture@example.com','test-only') RETURNING id").Scan(&otherUser); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetTrip(ctx, otherUser, id); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("foreign trip exposed", err)
	}
	if _, err := store.GetTrip(ctx, user, "99999999-9999-9999-9999-999999999999"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("missing trip not handled", err)
	}
	otherTrips, _, err := store.ListTrips(ctx, otherUser, 0)
	if err != nil || len(otherTrips) != 0 {
		t.Fatal("foreign trips listed", err)
	}
	if _, err := owner.ExecContext(ctx, "UPDATE catalog_cities SET active=false WHERE id=$1", ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetTrip(ctx, user, id); err != nil {
		t.Fatal("inactive city hides saved trip", err)
	}
	if _, err := owner.ExecContext(ctx, "UPDATE catalog_cities SET active=true WHERE id=$1", ids[0]); err != nil {
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
	// Enough records for two pages; identical dates exercise the ID tiebreaker.
	if _, err := owner.ExecContext(ctx, `INSERT INTO trip_requests(user_id,request_id,origin_id,destination_id,departure_from,departure_to,adults)
 SELECT $1,gen_random_uuid(),$2,$3,'2027-01-01','2027-01-02',1 FROM generate_series(1,20)`, user, ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	first, more, err := store.ListTrips(ctx, user, 0)
	if err != nil || len(first) != 20 || !more {
		t.Fatal("first page", err)
	}
	second, more, err := store.ListTrips(ctx, user, 20)
	if err != nil || len(second) != 1 || more {
		t.Fatal("second page", err)
	}
	for _, item := range first {
		if item.ID == second[0].ID {
			t.Fatal("overlapping pages")
		}
	}

}
