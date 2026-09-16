package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routeexperiment"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReadRoutes reads a stable snapshot and slices JSON arrays in PostgreSQL.
// It never runs a planner or reads Cabinet's database.
func (s *Store) ReadRoutes(ctx context.Context, q *v1.GetRoutesRequest) (*v1.GetRoutesResponse, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res := &v1.GetRoutesResponse{}
	err = tx.QueryRowContext(ctx, `SELECT revision,stage FROM route_building WHERE request_id=$1`, q.RequestId).Scan(&res.Revision, &res.Stage)
	if errors.Is(err, sql.ErrNoRows) {
		return res, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `WITH source_data AS (
 SELECT planner_id,stage,outcome,attempt,started_at,finished_at,route_count,incomplete,result FROM planner_runs WHERE request_id=$1
 UNION ALL
 SELECT 'graph',stage,'',attempt,started_at,finished_at,CASE WHEN jsonb_typeof(result->'candidates')='array' THEN jsonb_array_length(result->'candidates') ELSE 0 END,true,result FROM route_building b WHERE request_id=$1 AND result IS NOT NULL AND planners IS NULL AND NOT EXISTS(SELECT 1 FROM planner_runs p WHERE p.request_id=b.request_id)
 ), arrays AS (
 SELECT *,CASE WHEN jsonb_typeof(CASE WHEN planner_id='graph' THEN result->'candidates' ELSE result->'paths' END)='array' THEN CASE WHEN planner_id='graph' THEN result->'candidates' ELSE result->'paths' END ELSE '[]'::jsonb END AS routes FROM source_data
 )
 SELECT planner_id,stage,outcome,attempt,started_at,finished_at,route_count,incomplete,COALESCE(result->'issues','[]'::jsonb),
 COALESCE((SELECT jsonb_agg(value ORDER BY ordinal) FROM (SELECT value,ordinal FROM jsonb_array_elements(routes) WITH ORDINALITY AS r(value,ordinal) ORDER BY ordinal OFFSET $3 LIMIT $4) AS page),'[]'::jsonb)
 FROM arrays WHERE ($2='' OR planner_id=$2) ORDER BY planner_id`, q.RequestId, q.PlannerId, q.Offset, q.PageSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		source := &v1.SourceRoutes{Offset: q.Offset}
		var started, finished sql.NullTime
		var raw, issues []byte
		if err = rows.Scan(&source.PlannerId, &source.Stage, &source.Outcome, &source.Attempt, &started, &finished, &source.Total, &source.Incomplete, &issues, &raw); err != nil {
			return nil, err
		}
		if started.Valid {
			source.StartedAt = timestamppb.New(started.Time)
		}
		if finished.Valid {
			source.FinishedAt = timestamppb.New(finished.Time)
			if started.Valid {
				source.DurationMs = max(0, finished.Time.Sub(started.Time).Milliseconds())
			}
		}
		if err = json.Unmarshal(issues, &source.Warnings); err != nil {
			return nil, errors.New("invalid saved source warnings")
		}
		if source.PlannerId == "graph" {
			var candidates []realroutes.Candidate
			if json.Unmarshal(raw, &candidates) != nil {
				return nil, errors.New("invalid saved graph result")
			}
			for _, c := range candidates {
				scheme := &v1.RouteScheme{Warnings: c.Warnings}
				for _, step := range c.Steps {
					text := step.From + " → " + step.To
					if step.Number != "" {
						text += " · " + step.Number
					}
					evidence := ""
					switch step.Evidence {
					case "assumed":
						evidence = "Предполагаемый переезд; доступность и длительность не проверены"
					case "yandex-rasp":
						evidence = "Связь найдена в Яндекс Расписаниях; даты и стыковки не проверены"
					default:
						evidence = "Транспортная связь требует проверки"
					}
					scheme.Steps = append(scheme.Steps, &v1.RouteStep{Description: text, Mode: step.Mode, Evidence: evidence})
				}
				source.Routes = append(source.Routes, scheme)
			}
		} else {
			var paths []routeexperiment.Path
			if json.Unmarshal(raw, &paths) != nil {
				return nil, errors.New("invalid saved Gemini result")
			}
			for _, p := range paths {
				scheme := &v1.RouteScheme{Warnings: p.Warnings}
				for _, step := range p.Steps {
					scheme.Steps = append(scheme.Steps, &v1.RouteStep{Description: step, Evidence: "Предложение модели; транспортная связь не подтверждена"})
				}
				source.Routes = append(source.Routes, scheme)
			}
		}
		// Be explicit if a previously stored counter doesn't match the selected page.
		if len(source.Routes) > 0 && int(source.Total) < int(q.Offset)+len(source.Routes) {
			return nil, fmt.Errorf("invalid saved %s count", strings.TrimSpace(source.PlannerId))
		}
		source.HasMore = int64(q.Offset)+int64(len(source.Routes)) < int64(source.Total)
		res.Sources = append(res.Sources, source)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}
