//go:build integration

package auth

import (
	"bytes"
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

func testTripLifecycle(t *testing.T, ctx context.Context, owner, app *sql.DB, user, origin, destination string) {
	t.Helper()
	store := storage.New(app)
	date := time.Now().AddDate(0, 0, 5).Format(time.DateOnly)
	trip := storage.Trip{UserID: user, RequestID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", OriginID: origin, DestinationID: destination, DepartureFrom: date, DepartureTo: date, Adults: 1}
	id, err := store.CreateTrip(ctx, trip)
	if err != nil {
		t.Fatal(err)
	}
	check, err := store.GetTrip(ctx, user, id)
	if err != nil || check.Status != "saved" || check.CancelledAt.Valid {
		t.Fatal("initial status", err)
	}
	foreign := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	if err := store.CancelTrip(ctx, foreign, id); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("foreign cancellation", err)
	}
	if err := store.UpdateTripComment(ctx, foreign, id, "private"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("foreign comment", err)
	}
	if err := store.UpdateTripComment(ctx, user, id, "Личная заметка <script>"); err != nil {
		t.Fatal(err)
	}
	// Inject an outbox error after the status UPDATE; both must roll back.
	if _, err := owner.ExecContext(ctx, "ALTER TABLE outbox_events ADD CONSTRAINT reject_cancellation CHECK (event_type <> 'travelwatch.events.v1.TripRequestCancelled') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelTrip(ctx, user, id); err == nil {
		t.Fatal("outbox failure accepted")
	}
	if _, err := owner.ExecContext(ctx, "ALTER TABLE outbox_events DROP CONSTRAINT reject_cancellation"); err != nil {
		t.Fatal(err)
	}
	check, err = store.GetTrip(ctx, user, id)
	if err != nil || check.Status != "saved" || check.CancelledAt.Valid {
		t.Fatal("partial cancellation committed", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = store.CancelTrip(ctx, user, id) })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var payload []byte
	var eventID string
	if err := app.QueryRowContext(ctx, "SELECT count(*) FROM outbox_events WHERE request_id=$1 AND event_type='travelwatch.events.v1.TripRequestCancelled'", id).Scan(&n); err != nil || n != 1 {
		t.Fatal("duplicate cancellation", n, err)
	}
	if err := app.QueryRowContext(ctx, "SELECT id,payload FROM outbox_events WHERE request_id=$1 AND event_type='travelwatch.events.v1.TripRequestCancelled'", id).Scan(&eventID, &payload); err != nil {
		t.Fatal(err)
	}
	var event eventsv1.TripRequestCancelled
	if err := proto.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	check, err = store.GetTrip(ctx, user, id)
	if err != nil || check.Status != "cancelled" || !check.CancelledAt.Valid || check.Comment != "Личная заметка <script>" || event.EventId != eventID || event.RequestId != id || event.SchemaVersion != 1 || event.OccurredAt == nil || !event.OccurredAt.AsTime().Equal(check.CancelledAt.Time) {
		t.Fatal("cancelled history/event", err)
	}
	if _, err := store.CreateTrip(ctx, trip); err != nil {
		t.Fatal("creation retry failed", err)
	}
	if err := store.CancelTrip(ctx, user, id); err != nil {
		t.Fatal(err)
	}
	var after []byte
	if err := app.QueryRowContext(ctx, "SELECT payload FROM outbox_events WHERE id=$1", eventID).Scan(&after); err != nil || !bytes.Equal(payload, after) {
		t.Fatal("retry changed cancellation", err)
	}
	check, err = store.GetTrip(ctx, user, id)
	if err != nil || check.Status != "cancelled" {
		t.Fatal("creation retry reactivated trip", err)
	}
	if err := store.UpdateTripComment(ctx, user, id, ""); err != nil {
		t.Fatal(err)
	}
	check, err = store.GetTrip(ctx, user, id)
	if err != nil || check.Comment != "" || check.Status != "cancelled" {
		t.Fatal("comment edit changed status", err)
	}
	// Future completed states cannot be cancelled through the public API.
	trip.RequestID = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	complete, err := store.CreateTrip(ctx, trip)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ExecContext(ctx, "UPDATE trip_requests SET status='completed' WHERE id=$1", complete); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelTrip(ctx, user, complete); !errors.Is(err, storage.ErrTripTerminal) {
		t.Fatal("completed trip cancelled", err)
	}
}
