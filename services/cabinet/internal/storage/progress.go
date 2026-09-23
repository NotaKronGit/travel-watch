package storage

import (
	"bytes"
	"context"
	"errors"
	contract "github.com/NotaKronGit/travel-watch/api/progress"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"google.golang.org/protobuf/proto"
)

func (s *Store) ApplyProgress(ctx context.Context, e *eventsv1.TripRouteBuildingUpdated, payload []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM trip_requests WHERE id=$1 FOR UPDATE`, e.RequestId).Scan(&status); err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO trip_stage_history(event_id,request_id,revision,payload) VALUES($1,$2,$3,$4)
 ON CONFLICT DO NOTHING`, e.EventId, e.RequestId, e.Revision, payload)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var old []byte
		err = tx.QueryRowContext(ctx, `SELECT payload FROM trip_stage_history WHERE event_id=$1 AND request_id=$2 AND revision=$3`, e.EventId, e.RequestId, e.Revision).Scan(&old)
		if err != nil || !bytes.Equal(old, payload) {
			return errors.New("progress identity conflict")
		}
		return tx.Commit()
	}
	// History may arrive out of order; projection only advances. Local cancellation and expiry are terminal.
	if status != "cancelled" && status != "completed" && status != "expired" && (e.SchemaVersion == 1 || e.PlannerId == "all") {
		next := status
		if contract.Stages[e.Stage] != "queued" {
			next = "running"
		}
		if contract.Stages[e.Stage] == "cancelled" {
			return errors.New("Search cancellation without Cabinet cancellation")
		}
		_, err = tx.ExecContext(ctx, `UPDATE trip_requests SET building_stage=$2,building_revision=$3,status=$4
 WHERE id=$1 AND building_revision<$3`, e.RequestId, contract.Stages[e.Stage], e.Revision, next)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) tripHistory(ctx context.Context, id string) ([]*eventsv1.TripRouteBuildingUpdated, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM trip_stage_history WHERE request_id=$1 ORDER BY revision`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := []*eventsv1.TripRouteBuildingUpdated{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		e := new(eventsv1.TripRouteBuildingUpdated)
		if err = proto.Unmarshal(b, e); err != nil {
			return nil, err
		}
		history = append(history, e)
	}
	return history, rows.Err()
}
