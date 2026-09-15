// Package storage owns Search's database and atomic inbox processing.
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"

	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
)

var postgres = goqu.Dialect("postgres")
var ErrConflict = errors.New("event identity reused with different contents")

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Apply(ctx context.Context, e consumer.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	digest := sha256.Sum256(e.Payload)
	q, args, err := postgres.Insert("inbox_events").Rows(goqu.Record{"event_id": e.ID, "request_id": e.RequestID, "event_type": e.Type, "payload_hash": digest[:]}).OnConflict(goqu.DoNothing()).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var requestID, kind string
		var hash []byte
		if err = tx.QueryRowContext(ctx, "SELECT request_id,event_type,payload_hash FROM inbox_events WHERE event_id=$1", e.ID).Scan(&requestID, &kind, &hash); err != nil {
			return err
		}
		if requestID != e.RequestID || kind != e.Type || !bytes.Equal(hash, digest[:]) {
			return ErrConflict
		}
		return tx.Commit()
	}
	switch e.Type {
	case consumer.Created:
		// Only enrich a cancellation tombstone; never reactivate it or replace a snapshot.
		_, err = tx.ExecContext(ctx, `INSERT INTO search_requests(request_id,status,created_payload,created_at)
   VALUES($1,'pending',$2,$3) ON CONFLICT(request_id) DO UPDATE
   SET created_payload=EXCLUDED.created_payload,created_at=EXCLUDED.created_at
   WHERE search_requests.created_payload IS NULL`, e.RequestID, e.Payload, e.OccurredAt)
		if err == nil {
			var payload []byte
			err = tx.QueryRowContext(ctx, "SELECT created_payload FROM search_requests WHERE request_id=$1", e.RequestID).Scan(&payload)
			if err == nil && !bytes.Equal(payload, e.Payload) {
				err = ErrConflict
			}
		}
	case consumer.Cancelled:
		_, err = tx.ExecContext(ctx, `INSERT INTO search_requests(request_id,status,cancelled_at)
   VALUES($1,'cancelled',$2) ON CONFLICT(request_id) DO UPDATE
   SET status='cancelled',cancelled_at=COALESCE(search_requests.cancelled_at,EXCLUDED.cancelled_at)`, e.RequestID, e.OccurredAt)
	default:
		return consumer.ErrInvalidEvent
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
