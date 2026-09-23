package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/doug-martin/goqu/v9"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ErrTripTerminal = errors.New("trip already completed")

func (s *Store) CancelTrip(ctx context.Context, user, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	// Lock by owner and ID to serialize concurrent cancellations and future state transitions.
	err = tx.QueryRowContext(ctx, "SELECT status FROM trip_requests WHERE user_id=$1 AND id=$2 FOR UPDATE", user, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "cancelled" {
		return nil
	}
	if status == "completed" || status == "expired" {
		return ErrTripTerminal
	}
	q, args, err := postgres.Update("trip_requests").Set(goqu.Record{"status": "cancelled", "cancelled_at": goqu.L("clock_timestamp()")}).Where(goqu.Ex{"user_id": user, "id": id}).Returning("cancelled_at", goqu.L("gen_random_uuid()")).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	var cancelled time.Time
	var eventID string
	if err := tx.QueryRowContext(ctx, q, args...).Scan(&cancelled, &eventID); err != nil {
		return err
	}
	payload, err := proto.Marshal(&eventsv1.TripRequestCancelled{EventId: eventID, SchemaVersion: 1, RequestId: id, OccurredAt: timestamppb.New(cancelled)})
	if err != nil {
		return err
	}
	q, args, err = postgres.Insert("outbox_events").Rows(goqu.Record{"id": eventID, "request_id": id, "event_type": "travelwatch.events.v1.TripRequestCancelled", "payload": payload}).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateTripComment(ctx context.Context, user, id, comment string) error {
	q, args, err := postgres.Update("trip_requests").Set(goqu.Record{"comment": comment}).Where(goqu.Ex{"user_id": user, "id": id}).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
