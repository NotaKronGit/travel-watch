package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/NotaKronGit/travel-watch/api/progress"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/planning"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

// Called under the route_building row lock; event and state commit together.
func progressEvent(ctx context.Context, tx *sql.Tx, id string, count int, incomplete bool) error {
	var stage string
	var rev int64
	var attempt int32
	var started, finished sql.NullTime
	var now time.Time
	if err := tx.QueryRowContext(ctx, `UPDATE route_building SET revision=revision+1
 WHERE request_id=$1
 RETURNING stage,revision,attempt,started_at,finished_at,clock_timestamp()`, id).Scan(&stage, &rev, &attempt, &started, &finished, &now); err != nil {
		return err
	}
	if attempt < 0 || attempt > 100 || count < 0 || count > 100 {
		return errors.New("invalid progress counters")
	}
	e := &eventsv1.TripRouteBuildingUpdated{PlannerId: "all", EventId: uuid.NewString(), SchemaVersion: 2, RequestId: id, Revision: rev, Attempt: attempt, OccurredAt: timestamppb.New(now), RouteCount: int32(count), Incomplete: incomplete}
	for v, name := range progress.Stages {
		if name == stage {
			e.Stage = v
		}
	}
	if started.Valid {
		e.StartedAt = timestamppb.New(started.Time)
	}
	if finished.Valid {
		e.FinishedAt = timestamppb.New(finished.Time)
		e.DurationMs = max(0, finished.Time.Sub(started.Time).Milliseconds())
	}
	b, err := proto.Marshal(e)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO progress_outbox(event_id,request_id,revision,payload) VALUES($1,$2,$3,$4)`, e.EventId, id, rev, b)
	return err
}
func (s *Store) ClaimBuilding(ctx context.Context, lease time.Duration, maxAttempts int) (planning.Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return planning.Job{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	j := planning.Job{Token: uuid.NewString()}
	err = tx.QueryRowContext(ctx, `SELECT b.request_id,r.created_payload,b.attempt
 FROM route_building b
 JOIN search_requests r USING(request_id)
 WHERE r.status='pending' AND (b.stage='queued' OR (b.stage='building' AND b.lease_until<now()))
 ORDER BY b.created_at,b.request_id FOR UPDATE OF b SKIP LOCKED LIMIT 1`).Scan(&j.RequestID, &j.Payload, &j.Attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return j, false, nil
	}
	if err != nil {
		return j, false, err
	}
	if len(s.planners) > 0 {
		members, marshalErr := json.Marshal(s.planners)
		if marshalErr != nil {
			return j, false, marshalErr
		}
		if _, err = tx.ExecContext(ctx, `UPDATE route_building SET planners=COALESCE(planners,$2::jsonb) WHERE request_id=$1`, j.RequestID, string(members)); err != nil {
			return j, false, err
		}
	}
	if j.Attempt >= maxAttempts {
		var multi bool
		if err = tx.QueryRowContext(ctx, `SELECT planners IS NOT NULL FROM route_building WHERE request_id=$1`, j.RequestID).Scan(&multi); err != nil {
			return j, false, err
		}
		if multi {
			if err = exhaustSources(ctx, tx, j.RequestID); err != nil {
				return j, false, err
			}
			return j, false, tx.Commit()
		}
		_, err = tx.ExecContext(ctx, `UPDATE route_building SET stage='failed',finished_at=GREATEST(clock_timestamp(),started_at),lease_token=NULL,lease_until=NULL
 WHERE request_id=$1`, j.RequestID)
		if err == nil {
			err = progressEvent(ctx, tx, j.RequestID, 0, true)
		}
		if err == nil {
			err = tx.Commit()
		}
		return j, false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE route_building SET stage='building',attempt=attempt+1,started_at=clock_timestamp(),finished_at=NULL,lease_token=$2,lease_until=now()+$3::interval
 WHERE request_id=$1`, j.RequestID, j.Token, lease.String())
	if err == nil {
		err = progressEvent(ctx, tx, j.RequestID, 0, true)
	}
	if err == nil {
		err = s.startSources(ctx, tx, &j)
	}
	if err == nil {
		err = tx.Commit()
	}
	j.Attempt++
	return j, err == nil, err
}
func (s *Store) BuildingActive(ctx context.Context, j planning.Job) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1
 FROM route_building b
 JOIN search_requests r USING(request_id)
 WHERE b.request_id=$1 AND b.lease_token=$2 AND b.lease_until>now() AND b.stage='building' AND r.status='pending')`, j.RequestID, j.Token).Scan(&ok)
	return ok, err
}
func (s *Store) FinishBuilding(ctx context.Context, j planning.Job, stage string, result realroutes.Result) error {
	if stage != "awaiting_schedules" && stage != "no_routes" && stage != "failed" {
		return errors.New("invalid building completion")
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if len(b) > 1<<20 {
		return errors.New("route result too large")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Lock request first, as cancellation does, to ensure cancellation always wins.
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM search_requests WHERE request_id=$1 FOR UPDATE`, j.RequestID).Scan(&status); err != nil {
		return err
	}
	// Cancelled or expired: late results are dropped.
	if status != "pending" {
		return nil
	}
	r, err := tx.ExecContext(ctx, `UPDATE route_building SET stage=$3,result=$4,finished_at=GREATEST(clock_timestamp(),started_at),lease_token=NULL,lease_until=NULL
 WHERE request_id=$1 AND lease_token=$2 AND lease_until>now() AND stage='building'`, j.RequestID, j.Token, stage, string(b))
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	count := len(result.Candidates)
	if stage == "failed" {
		count = 0
	}
	if err = progressEvent(ctx, tx, j.RequestID, count, !result.Complete); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ClaimProgress(ctx context.Context, lease time.Duration) (planning.Message, bool, error) {
	m := planning.Message{Token: uuid.NewString()}
	err := s.db.QueryRowContext(ctx, `WITH candidate AS (
 SELECT event_id
 FROM progress_outbox
 WHERE published_at IS NULL AND (lease_until IS NULL OR lease_until<now())
 ORDER BY request_id,revision FOR UPDATE SKIP LOCKED LIMIT 1
 )
 UPDATE progress_outbox SET lease_token=$1,lease_until=now()+$2::interval
 FROM candidate
 WHERE progress_outbox.event_id=candidate.event_id
 RETURNING progress_outbox.event_id,request_id,payload`, m.Token, lease.String()).Scan(&m.ID, &m.RequestID, &m.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return m, false, nil
	}
	return m, err == nil, err
}
func (s *Store) AckProgress(ctx context.Context, m planning.Message) error {
	_, err := s.db.ExecContext(ctx, `UPDATE progress_outbox SET published_at=now(),lease_token=NULL,lease_until=NULL
 WHERE event_id=$1 AND lease_token=$2 AND lease_until>now()`, m.ID, m.Token)
	return err
}
