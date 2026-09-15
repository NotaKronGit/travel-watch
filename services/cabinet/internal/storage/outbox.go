package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/doug-martin/goqu/v9"
)

type OutboxMessage struct {
	ID, RequestID, LeaseToken, EventType string
	Payload                              []byte
	Attempts                             int
}

// ClaimOutbox commits a short row lease before any network publication.
func (s *Store) ClaimOutbox(ctx context.Context, lease time.Duration) (OutboxMessage, bool, error) {
	var m OutboxMessage
	err := s.db.QueryRowContext(ctx, `WITH candidate AS (
 SELECT id FROM outbox_events WHERE published_at IS NULL AND available_at <= now()
 AND (lease_until IS NULL OR lease_until <= now())
 ORDER BY available_at, created_at, id FOR UPDATE SKIP LOCKED LIMIT 1
 ) UPDATE outbox_events e SET lease_until=now()+($1 * interval '1 millisecond'),
 lease_token=gen_random_uuid(), attempts=CASE WHEN attempts<2147483647 THEN attempts+1 ELSE attempts END
 FROM candidate c WHERE e.id=c.id RETURNING e.id,e.request_id,e.payload,e.lease_token,e.attempts,e.event_type`, lease.Milliseconds()).Scan(&m.ID, &m.RequestID, &m.Payload, &m.LeaseToken, &m.Attempts, &m.EventType)
	if errors.Is(err, sql.ErrNoRows) {
		return OutboxMessage{}, false, nil
	}
	return m, err == nil, err
}
func (s *Store) finishOutbox(ctx context.Context, m OutboxMessage, values goqu.Record) error {
	q, args, err := postgres.Update("outbox_events").Set(values).Where(goqu.Ex{"id": m.ID, "lease_token": m.LeaseToken, "published_at": nil}).Prepared(true).ToSQL()
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
	if n != 1 {
		return errors.New("outbox lease no longer owned")
	}
	return nil
}
func (s *Store) AckOutbox(ctx context.Context, m OutboxMessage) error {
	return s.finishOutbox(ctx, m, goqu.Record{"published_at": goqu.L("now()"), "lease_token": nil, "lease_until": nil, "last_error": ""})
}
func (s *Store) RetryOutbox(ctx context.Context, m OutboxMessage, delay time.Duration) error {
	return s.finishOutbox(ctx, m, goqu.Record{"available_at": goqu.L("now()+(? * interval '1 millisecond')", delay.Milliseconds()), "lease_token": nil, "lease_until": nil, "last_error": "publish_failed"})
}
