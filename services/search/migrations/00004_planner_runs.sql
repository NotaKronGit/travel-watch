-- +goose Up
ALTER TABLE route_building ADD COLUMN planners jsonb;
CREATE TABLE planner_runs (
 request_id uuid NOT NULL REFERENCES route_building(request_id),
 planner_id text NOT NULL CHECK (planner_id IN ('graph','gemini')),
 stage text NOT NULL CHECK (stage IN ('building','awaiting_schedules','no_routes','failed','cancelled')),
 attempt integer NOT NULL,
 started_at timestamptz NOT NULL,
 finished_at timestamptz,
 route_count integer NOT NULL DEFAULT 0 CHECK (route_count BETWEEN 0 AND 50),
 incomplete boolean NOT NULL DEFAULT true,
 outcome text NOT NULL DEFAULT '',
 result jsonb,
 PRIMARY KEY(request_id,planner_id)
);
-- Preserve membership of an already running legacy job during upgrade.
UPDATE route_building SET planners='["graph"]'::jsonb WHERE stage='building';
INSERT INTO planner_runs(request_id,planner_id,stage,attempt,started_at)
 SELECT request_id,'graph','building',attempt,started_at FROM route_building WHERE stage='building';
GRANT SELECT, INSERT, UPDATE ON planner_runs TO search_app;
-- +goose Down
DROP TABLE planner_runs;
ALTER TABLE route_building DROP COLUMN planners;
