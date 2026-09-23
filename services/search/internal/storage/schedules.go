package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/schedules"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
)

// ClaimSchedules also picks up existing completed graph results. Request-row
// locking serializes claims with cancellation. A lease token fences late workers.
func (s *Store) ClaimSchedules(ctx context.Context, lease time.Duration) (schedules.Job, bool, error) {
	j := schedules.Job{Token: uuid.NewString()}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return j, false, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `UPDATE schedule_checks SET state='failed',finished_at=now(),lease_token=NULL,lease_until=NULL WHERE state='running' AND lease_until<now() AND attempt>=3`)
	if err != nil {
		return j, false, err
	}
	err = tx.QueryRowContext(ctx, `SELECT r.request_id,r.created_payload,p.result,p.finished_at
 FROM search_requests r JOIN planner_runs p ON p.request_id=r.request_id AND p.planner_id='graph'
 LEFT JOIN schedule_checks c ON c.request_id=r.request_id
 WHERE r.status='pending' AND p.stage='awaiting_schedules' AND p.result IS NOT NULL
 AND (c.request_id IS NULL OR (c.state='running' AND c.lease_until<now() AND c.attempt<3))
 ORDER BY p.finished_at,r.request_id FOR UPDATE OF r SKIP LOCKED LIMIT 1`).Scan(&j.RequestID, &j.Payload, &j.Graph, &j.SourceFinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return j, false, tx.Commit()
	}
	if err != nil {
		return j, false, err
	}
	claimed, err := tx.ExecContext(ctx, `INSERT INTO schedule_checks(request_id,state,source_finished_at,lease_token,lease_until) VALUES($1,'running',$2,$3,now()+$4::interval)
 ON CONFLICT(request_id) DO UPDATE SET state='running',attempt=schedule_checks.attempt+1,lease_token=EXCLUDED.lease_token,lease_until=EXCLUDED.lease_until,source_finished_at=EXCLUDED.source_finished_at,started_at=now(),finished_at=NULL WHERE schedule_checks.state='running' AND schedule_checks.lease_until<now() AND schedule_checks.attempt<3`, j.RequestID, j.SourceFinishedAt, j.Token, lease.String())
	if err != nil {
		return j, false, err
	}
	n, err := claimed.RowsAffected()
	if err != nil {
		return j, false, err
	}
	err = tx.Commit()
	return j, err == nil && n == 1, err
}
func (s *Store) SchedulesActive(ctx context.Context, j schedules.Job) (bool, error) {
	var active bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schedule_checks c JOIN search_requests r USING(request_id) JOIN planner_runs p ON p.request_id=c.request_id AND p.planner_id='graph' WHERE c.request_id=$1 AND c.lease_token=$2 AND c.lease_until>now() AND c.state='running' AND r.status='pending' AND p.finished_at=c.source_finished_at)`, j.RequestID, j.Token).Scan(&active)
	return active, err
}
func (s *Store) FinishSchedules(ctx context.Context, j schedules.Job, result *v1.ScheduleCheck) error {
	if result == nil || (result.State != "done" && result.State != "failed") {
		return errors.New("invalid schedule result")
	}
	data, err := protojson.Marshal(result)
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("schedule result too large")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM search_requests WHERE request_id=$1 FOR UPDATE`, j.RequestID).Scan(&status); err != nil {
		return err
	}
	// Cancelled or expired: late results are dropped.
	if status != "pending" {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE schedule_checks c SET state=$3,result=$4,finished_at=now(),lease_token=NULL,lease_until=NULL WHERE request_id=$1 AND lease_token=$2 AND lease_until>now() AND state='running' AND EXISTS(SELECT 1 FROM planner_runs p WHERE p.request_id=c.request_id AND p.planner_id='graph' AND p.finished_at=c.source_finished_at)`, j.RequestID, j.Token, result.State, string(data))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func readSchedules(ctx context.Context, tx *sql.Tx, id string) (*v1.ScheduleCheck, error) {
	var raw []byte
	var state, status string
	err := tx.QueryRowContext(ctx, `SELECT c.state,r.status,c.result FROM schedule_checks c JOIN search_requests r USING(request_id) WHERE c.request_id=$1`, id).Scan(&state, &status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &v1.ScheduleCheck{State: "pending"}, nil
	}
	if err != nil {
		return nil, err
	}
	result := &v1.ScheduleCheck{State: state}
	if len(raw) > 0 {
		if err = protojson.Unmarshal(raw, result); err != nil {
			return nil, errors.New("invalid saved schedule result")
		}
	}
	if status == "cancelled" {
		result.State = "cancelled"
	}
	return result, nil
}
