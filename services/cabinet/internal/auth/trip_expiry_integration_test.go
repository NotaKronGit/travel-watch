//go:build integration

package auth

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"google.golang.org/protobuf/proto"
)

// Runs last: it expires every active fixture trip dated before the chosen instant.
func testTripExpiry(t *testing.T, ctx context.Context, owner, app *sql.DB, user, origin, destination string) {
	t.Helper()
	store := storage.New(app)
	// Synthetic city whose timezone PostgreSQL does not know: falls back to UTC.
	unknownTZ := "55555555-5555-5555-5555-555555555555"
	if _, err := owner.ExecContext(ctx, "INSERT INTO catalog_cities(id,source,source_id,name,name_ru,country_code,region_code,timezone,aliases,latitude,longitude,population,active) VALUES($1,'trip-test',99,'Fixture','Тестовый город','RU','01','Test/Unknown','[]',0,0,0,true)", unknownTZ); err != nil {
		t.Fatal(err)
	}
	insert := func(originID, to, status string) string {
		var id string
		cancelled := sql.NullTime{Valid: status == "cancelled", Time: time.Now()}
		if err := owner.QueryRowContext(ctx, `INSERT INTO trip_requests(user_id,request_id,origin_id,destination_id,departure_from,departure_to,adults,status,cancelled_at)
 VALUES($1,gen_random_uuid(),$2,$3,$4,$4,1,$5,$6) RETURNING id`, user, originID, destination, to, status, cancelled).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	moscow := insert(origin, "2032-01-10", "running")   // Europe/Moscow, UTC+3
	later := insert(origin, "2032-01-11", "saved")      // one day later
	unknown := insert(unknownTZ, "2032-01-10", "saved") // compared in UTC
	cancelled := insert(origin, "2032-01-01", "cancelled")
	status := func(id string) string {
		var s string
		if err := app.QueryRowContext(ctx, "SELECT status FROM trip_requests WHERE id=$1", id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// 23:30 on Jan 10 in Moscow: the last departure day is not over yet anywhere.
	if _, err := store.ExpireTrips(ctx, at("2032-01-10T20:30:00Z"), 1000); err != nil || status(moscow) != "running" || status(unknown) != "saved" {
		t.Fatal("expired before the day ended", err)
	}
	// 00:30 on Jan 11 in Moscow, still Jan 10 in UTC.
	if _, err := store.ExpireTrips(ctx, at("2032-01-10T21:30:00Z"), 1000); err != nil || status(moscow) != "expired" || status(unknown) != "saved" || status(later) != "saved" {
		t.Fatal("origin timezone boundary", err, status(moscow), status(unknown))
	}
	// Concurrent passes after Jan 10 is over in UTC too: one event per trip.
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { _, errs[i] = store.ExpireTrips(ctx, at("2032-01-11T00:30:00Z"), 1000) })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if status(unknown) != "expired" || status(later) != "saved" || status(cancelled) != "cancelled" {
		t.Fatal("unknown timezone or unrelated trips", status(unknown), status(later), status(cancelled))
	}
	for _, id := range []string{moscow, unknown} {
		var n int
		var payload []byte
		if err := app.QueryRowContext(ctx, "SELECT count(*) FROM outbox_events WHERE request_id=$1 AND event_type='travelwatch.events.v1.TripRequestExpired'", id).Scan(&n); err != nil || n != 1 {
			t.Fatal("expiry events", id, n, err)
		}
		if err := app.QueryRowContext(ctx, "SELECT payload FROM outbox_events WHERE request_id=$1 AND event_type='travelwatch.events.v1.TripRequestExpired'", id).Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var event eventsv1.TripRequestExpired
		if err := proto.Unmarshal(payload, &event); err != nil || event.RequestId != id || event.SchemaVersion != 1 || event.OccurredAt == nil {
			t.Fatal("expiry payload", err)
		}
		var expiredAt sql.NullTime
		if err := app.QueryRowContext(ctx, "SELECT expired_at FROM trip_requests WHERE id=$1", id).Scan(&expiredAt); err != nil || !expiredAt.Valid {
			t.Fatal("expired_at", err)
		}
	}
	var n int
	if err := app.QueryRowContext(ctx, "SELECT count(*) FROM outbox_events WHERE request_id=$1", cancelled).Scan(&n); err != nil || n != 0 {
		t.Fatal("cancelled trip got an event", n, err)
	}
	if err := store.CancelTrip(ctx, user, moscow); !errors.Is(err, storage.ErrTripTerminal) {
		t.Fatal("expired trip cancelled", err)
	}
	trips, _, err := store.ListTrips(ctx, user, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, trip := range trips {
		if trip.ID == moscow || trip.ID == unknown {
			t.Fatal("expired trip listed as active")
		}
	}
}
