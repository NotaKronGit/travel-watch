-- +goose Up
CREATE TABLE route_building (
 request_id uuid PRIMARY KEY REFERENCES search_requests(request_id),
 stage text NOT NULL CHECK (stage IN ('queued','building','awaiting_schedules','no_routes','failed','cancelled')),
 revision bigint NOT NULL DEFAULT 0,
 attempt integer NOT NULL DEFAULT 0,
 lease_token uuid,
 lease_until timestamptz,
 started_at timestamptz,
 finished_at timestamptz,
 result jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX route_building_pending ON route_building(created_at) WHERE stage IN ('queued','building');
CREATE TABLE progress_outbox (
 event_id uuid PRIMARY KEY,
 request_id uuid NOT NULL REFERENCES search_requests(request_id),
 revision bigint NOT NULL,
 payload bytea NOT NULL,
 published_at timestamptz,
 lease_token uuid,
 lease_until timestamptz,
 UNIQUE(request_id, revision)
);
GRANT SELECT, INSERT, UPDATE ON route_building, progress_outbox TO search_app;
-- Existing requests are deliberately not enqueued by migration.
-- +goose Down
DROP TABLE progress_outbox;
DROP TABLE route_building;
