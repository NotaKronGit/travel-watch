package storage

import (
	"context"
	"errors"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/doug-martin/goqu/v9"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ExpireTrips moves up to limit active trips whose last departure day is over in the
// origin city's timezone to expired, each with its outbox event, and returns how many
// changed. A timezone PostgreSQL does not know falls back to UTC so no trip stays
// active forever. Safe to run from several processes: every trip is re-checked under
// its row lock, as cancellation does, so it expires and emits an event only once.
func (s *Store) ExpireTrips(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit < 1 {
		return 0, errors.New("expiry batch must be positive")
	}
	ids, err := s.expiryCandidates(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, id := range ids {
		changed, err := s.expireTrip(ctx, id)
		if err != nil {
			return expired, err
		}
		if changed {
			expired++
		}
	}
	return expired, nil
}

// expiryCandidates reads the batch and releases its connection before the per-trip transactions.
func (s *Store) expiryCandidates(ctx context.Context, now time.Time, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id
 FROM trip_requests t
 JOIN catalog_cities c ON c.id=t.origin_id
 LEFT JOIN pg_timezone_names z ON z.name=c.timezone
 WHERE t.status IN ('saved','running') AND t.departure_to<($1::timestamptz AT TIME ZONE COALESCE(z.name,'UTC'))::date
 ORDER BY t.departure_to,t.id LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) expireTrip(ctx context.Context, id string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM trip_requests WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return false, err
	}
	// Cancelled or completed meanwhile, or expired by another process.
	if status != "saved" && status != "running" {
		return false, nil
	}
	q, args, err := postgres.Update("trip_requests").Set(goqu.Record{"status": "expired", "expired_at": goqu.L("clock_timestamp()")}).Where(goqu.Ex{"id": id}).Returning("expired_at", goqu.L("gen_random_uuid()")).Prepared(true).ToSQL()
	if err != nil {
		return false, err
	}
	var at time.Time
	var eventID string
	if err = tx.QueryRowContext(ctx, q, args...).Scan(&at, &eventID); err != nil {
		return false, err
	}
	payload, err := proto.Marshal(&eventsv1.TripRequestExpired{EventId: eventID, SchemaVersion: 1, RequestId: id, OccurredAt: timestamppb.New(at)})
	if err != nil {
		return false, err
	}
	q, args, err = postgres.Insert("outbox_events").Rows(goqu.Record{"id": eventID, "request_id": id, "event_type": "travelwatch.events.v1.TripRequestExpired", "payload": payload}).Prepared(true).ToSQL()
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, q, args...); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
