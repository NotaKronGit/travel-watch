//go:build integration

package auth

import (
	"bytes"
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"google.golang.org/protobuf/proto"
)

func testTripOutbox(t *testing.T, ctx context.Context, owner, app *sql.DB, user, origin, destination string) {
	t.Helper()
	store := storage.New(app)
	date := time.Now().AddDate(0, 0, 5).Format(time.DateOnly)
	trip := storage.Trip{UserID: user, RequestID: "77777777-7777-7777-7777-777777777777", OriginID: origin, DestinationID: destination, DepartureFrom: date, DepartureTo: date, Adults: 2}
	t.Run("atomic snapshot and concurrent retries", func(t *testing.T) {
		var wg sync.WaitGroup
		ids := make([]string, 4)
		errs := make([]error, 4)
		for i := range ids {
			wg.Go(func() { ids[i], errs[i] = store.CreateTrip(ctx, trip) })
		}
		wg.Wait()
		for i := range ids {
			if errs[i] != nil || ids[i] != ids[0] {
				t.Fatalf("retry %d: id=%s error=%v", i, ids[i], errs[i])
			}
		}
		var eventID, kind string
		var payload []byte
		var created time.Time
		var count int
		if err := app.QueryRowContext(ctx, "SELECT count(*) FROM outbox_events WHERE request_id=$1", ids[0]).Scan(&count); err != nil || count != 1 {
			t.Fatal("duplicate event", count, err)
		}
		if err := app.QueryRowContext(ctx, "SELECT e.id,e.event_type,e.payload,t.created_at FROM outbox_events e JOIN trip_requests t ON t.id=e.request_id WHERE e.request_id=$1", ids[0]).Scan(&eventID, &kind, &payload, &created); err != nil {
			t.Fatal(err)
		}
		var event eventsv1.TripRequestCreated
		if err := proto.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		if kind != "travelwatch.events.v1.TripRequestCreated" || event.EventId != eventID || event.RequestId != ids[0] || event.SchemaVersion != 1 || event.Adults != 2 || event.DepartureFrom != date || event.DepartureTo != date || event.OccurredAt == nil || !event.OccurredAt.AsTime().Equal(created) {
			t.Fatalf("incorrect envelope: %v", &event)
		}
		if event.Origin == nil || event.Destination == nil || event.Origin.Id != origin || event.Destination.Id != destination || event.Origin.Source != "trip-test" || event.Origin.SourceId != 1 || event.Origin.Name != "Тестовый город" || event.Origin.CountryCode != "RU" || event.Origin.Timezone != "Europe/Moscow" {
			t.Fatalf("incorrect city snapshot: %v", &event)
		}
		if _, err := owner.ExecContext(ctx, "UPDATE catalog_cities SET name_ru='Изменённый город',latitude=12.3 WHERE id=$1", origin); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateTrip(ctx, trip); err != nil {
			t.Fatal(err)
		}
		var after []byte
		if err := app.QueryRowContext(ctx, "SELECT payload FROM outbox_events WHERE request_id=$1", ids[0]).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(payload, after) {
			t.Fatal("retry changed stored event")
		}
		changed := trip
		changed.Adults = 3
		if _, err := store.CreateTrip(ctx, changed); err == nil {
			t.Fatal("conflicting retry accepted")
		}
		if err := app.QueryRowContext(ctx, "SELECT count(*) FROM outbox_events WHERE request_id=$1", ids[0]).Scan(&count); err != nil || count != 1 {
			t.Fatal("conflicting retry changed events", err)
		}
	})
	t.Run("event failure rolls back trip", func(t *testing.T) {
		// A temporary constraint rejects the event after the trip INSERT in this isolated schema.
		if _, err := owner.ExecContext(ctx, "ALTER TABLE outbox_events ADD CONSTRAINT reject_new_events CHECK (false) NOT VALID"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := owner.ExecContext(ctx, "ALTER TABLE outbox_events DROP CONSTRAINT reject_new_events"); err != nil {
				t.Error(err)
			}
		}()
		attempt := trip
		attempt.RequestID = "88888888-8888-8888-8888-888888888888"
		if _, err := store.CreateTrip(ctx, attempt); err == nil {
			t.Fatal("expected outbox failure")
		}
		var count int
		if err := app.QueryRowContext(ctx, "SELECT count(*) FROM trip_requests WHERE user_id=$1 AND request_id=$2", user, attempt.RequestID).Scan(&count); err != nil || count != 0 {
			t.Fatal("trip committed without event", err)
		}
	})
	t.Run("existing request is not backfilled", func(t *testing.T) {
		old := trip
		old.RequestID = "99999999-9999-9999-9999-999999999999"
		var id string
		if err := owner.QueryRowContext(ctx, "INSERT INTO trip_requests(user_id,request_id,origin_id,destination_id,departure_from,departure_to,adults) VALUES($1,$2,$3,$4,$5,$5,2) RETURNING id", user, old.RequestID, origin, destination, date).Scan(&id); err != nil {
			t.Fatal(err)
		}
		got, err := store.CreateTrip(ctx, old)
		if err != nil || got != id {
			t.Fatal("old retry failed", err)
		}
		var count int
		if err := app.QueryRowContext(ctx, "SELECT count(*) FROM outbox_events WHERE request_id=$1", id).Scan(&count); err != nil || count != 0 {
			t.Fatal("old request backfilled", err)
		}
	})
}

func testOutboxLeases(t *testing.T, ctx context.Context, owner, app *sql.DB) {
	t.Helper()
	// Restrict this isolated schema to one pending event to exercise competing claimers.
	if _, err := owner.ExecContext(ctx, "UPDATE outbox_events SET published_at=now() WHERE id<>(SELECT id FROM outbox_events ORDER BY id LIMIT 1)"); err != nil {
		t.Fatal(err)
	}
	store := storage.New(app)
	var wg sync.WaitGroup
	messages := make([]storage.OutboxMessage, 2)
	found := make([]bool, 2)
	errs := make([]error, 2)
	for i := range messages {
		wg.Go(func() { messages[i], found[i], errs[i] = store.ClaimOutbox(ctx, time.Minute) })
	}
	wg.Wait()
	var first storage.OutboxMessage
	count := 0
	for i := range messages {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if found[i] {
			first = messages[i]
			count++
		}
	}
	if count != 1 {
		t.Fatal("same event claimed concurrently", count)
	}
	if _, err := owner.ExecContext(ctx, "UPDATE outbox_events SET lease_until=now()-interval '1 second' WHERE id=$1", first.ID); err != nil {
		t.Fatal(err)
	}
	second, ok, err := store.ClaimOutbox(ctx, time.Minute)
	if err != nil || !ok || second.ID != first.ID || second.LeaseToken == first.LeaseToken || second.Attempts != first.Attempts+1 || !bytes.Equal(second.Payload, first.Payload) {
		t.Fatal("expired lease not recovered", err)
	}
	if err := store.AckOutbox(ctx, first); err == nil {
		t.Fatal("stale ack accepted")
	}
	if err := store.RetryOutbox(ctx, first, time.Second); err == nil {
		t.Fatal("stale retry accepted")
	}
	if err := store.RetryOutbox(ctx, second, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.ClaimOutbox(ctx, time.Minute); err != nil || ok {
		t.Fatal("backoff ignored", err)
	}
	if _, err := owner.ExecContext(ctx, "UPDATE outbox_events SET available_at=now()-interval '1 second' WHERE id=$1", first.ID); err != nil {
		t.Fatal(err)
	}
	third, ok, err := store.ClaimOutbox(ctx, time.Minute)
	if err != nil || !ok {
		t.Fatal("retry unavailable", err)
	}
	if err := store.AckOutbox(ctx, third); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.ClaimOutbox(ctx, time.Minute); err != nil || ok {
		t.Fatal("confirmed event claimed", err)
	}
}
