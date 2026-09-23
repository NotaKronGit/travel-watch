package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/NotaKronGit/travel-watch/api/progress"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/planning"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func NewBuilder(db *sql.DB, planners []string) *Store {
	return &Store{db: db, planners: append([]string(nil), planners...)}
}
func (s *Store) startSources(ctx context.Context, tx *sql.Tx, j *planning.Job) error {
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT planners FROM route_building WHERE request_id=$1`, j.RequestID).Scan(&raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	} // Legacy single-source worker.
	var members []string
	if err := json.Unmarshal(raw, &members); err != nil {
		return err
	}
	for _, id := range members {
		r, err := tx.ExecContext(ctx, `INSERT INTO planner_runs(request_id,planner_id,stage,attempt,started_at) VALUES($1,$2,'building',$3,clock_timestamp())
 ON CONFLICT(request_id,planner_id) DO UPDATE SET attempt=EXCLUDED.attempt,started_at=EXCLUDED.started_at
 WHERE planner_runs.stage='building'`, j.RequestID, id, j.Attempt+1)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		j.Sources = append(j.Sources, id)
		if err = sourceEvent(ctx, tx, j.RequestID, id); err != nil {
			return err
		}
	}
	return nil
}
func sourceEvent(ctx context.Context, tx *sql.Tx, requestID, id string) error {
	var stage string
	var started time.Time
	var finished sql.NullTime
	e := &eventsv1.TripRouteBuildingUpdated{SchemaVersion: 2, EventId: uuid.NewString(), RequestId: requestID, PlannerId: id}
	if err := tx.QueryRowContext(ctx, `SELECT stage,attempt,started_at,finished_at,route_count,incomplete,outcome
 FROM planner_runs
 WHERE request_id=$1 AND planner_id=$2`, requestID, id).Scan(&stage, &e.Attempt, &started, &finished, &e.RouteCount, &e.Incomplete, &e.Outcome); err != nil {
		return err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `UPDATE route_building SET revision=revision+1
 WHERE request_id=$1
 RETURNING revision,clock_timestamp()`, requestID).Scan(&e.Revision, &now); err != nil {
		return err
	}
	e.OccurredAt = timestamppb.New(now)
	e.StartedAt = timestamppb.New(started)
	if finished.Valid {
		e.FinishedAt = timestamppb.New(finished.Time)
		e.DurationMs = max(0, finished.Time.Sub(started).Milliseconds())
	}
	for value, name := range progress.Stages {
		if name == stage {
			e.Stage = value
		}
	}
	b, err := proto.Marshal(e)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO progress_outbox(event_id,request_id,revision,payload) VALUES($1,$2,$3,$4)`, e.EventId, requestID, e.Revision, b)
	return err
}
func (s *Store) FinishSource(ctx context.Context, j planning.Job, id string, r planning.SourceResult) error {
	if id != "graph" && id != "gemini" || r.Count < 0 || r.Count > 50 || len(r.Data) > 1<<20 || (len(r.Data) > 0 && !json.Valid(r.Data)) {
		return errors.New("invalid source result")
	}
	switch r.Outcome {
	case "", "error", "timeout", "unavailable":
	default:
		return errors.New("invalid source outcome")
	}
	stage := "awaiting_schedules"
	if r.Count == 0 {
		stage = "no_routes"
		if r.Outcome != "" {
			stage = "failed"
		}
	}
	if r.Outcome != "" {
		r.Incomplete = true
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
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT stage='building' AND lease_token=$2 AND lease_until>now()
 FROM route_building
 WHERE request_id=$1 FOR UPDATE`, j.RequestID, j.Token).Scan(&active); err != nil {
		return err
	}
	if !active {
		return nil
	}
	data := r.Data
	if len(data) == 0 {
		data = json.RawMessage(`null`)
	}
	update, err := tx.ExecContext(ctx, `UPDATE planner_runs SET stage=$3,finished_at=GREATEST(clock_timestamp(),started_at),route_count=$4,incomplete=$5,result=$6,outcome=$7
 WHERE request_id=$1 AND planner_id=$2 AND stage='building'`, j.RequestID, id, stage, r.Count, r.Incomplete, string(data), r.Outcome)
	if err != nil {
		return err
	}
	n, err := update.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	if err = sourceEvent(ctx, tx, j.RequestID, id); err != nil {
		return err
	}
	if err = finishAggregate(ctx, tx, j.RequestID); err != nil {
		return err
	}
	return tx.Commit()
}

// Called with the job locked, after a participant becomes terminal.
func finishAggregate(ctx context.Context, tx *sql.Tx, id string) error {
	var pending, count, failed int
	var incomplete bool
	var result []byte
	err := tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE stage='building'),COALESCE(sum(route_count),0),count(*) FILTER(WHERE stage='failed'),COALESCE(bool_or(incomplete),false),
 jsonb_object_agg(planner_id,jsonb_build_object('stage',stage,'outcome',outcome,'result',result,'route_count',route_count))
 FROM planner_runs
 WHERE request_id=$1`, id).Scan(&pending, &count, &failed, &incomplete, &result)
	if err != nil || pending > 0 {
		return err
	}
	stage := "awaiting_schedules"
	if count == 0 {
		stage = "no_routes"
		if failed > 0 {
			stage = "failed"
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE route_building SET stage=$2,result=$3,finished_at=GREATEST(clock_timestamp(),started_at),lease_token=NULL,lease_until=NULL
 WHERE request_id=$1`, id, stage, string(result))
	if err != nil {
		return err
	}
	return progressEvent(ctx, tx, id, count, incomplete || failed > 0)
}

func exhaustSources(ctx context.Context, tx *sql.Tx, id string) error {
	rows, err := tx.QueryContext(ctx, `UPDATE planner_runs SET stage='failed',outcome='timeout',incomplete=true,finished_at=GREATEST(clock_timestamp(),started_at)
 WHERE request_id=$1 AND stage='building'
 RETURNING planner_id`, id)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var source string
		if err = rows.Scan(&source); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, source)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, source := range ids {
		if err = sourceEvent(ctx, tx, id, source); err != nil {
			return err
		}
	}
	return finishAggregate(ctx, tx, id)
}

func cancelSources(ctx context.Context, tx *sql.Tx, id string) error {
	rows, err := tx.QueryContext(ctx, `UPDATE planner_runs SET stage='cancelled',finished_at=GREATEST(clock_timestamp(),started_at)
 WHERE request_id=$1 AND stage='building'
 RETURNING planner_id`, id)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var source string
		if err = rows.Scan(&source); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, source)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, source := range ids {
		if err = sourceEvent(ctx, tx, id, source); err != nil {
			return err
		}
	}
	return nil
}
