-- +goose Up
CREATE TABLE schedule_checks (
 request_id uuid PRIMARY KEY REFERENCES search_requests(request_id),
 state text NOT NULL CHECK(state IN ('running','done','failed')),
 attempt integer NOT NULL DEFAULT 1,
 source_finished_at timestamptz NOT NULL,
 lease_token uuid,
 lease_until timestamptz,
 started_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz,
 result jsonb
);
CREATE INDEX schedule_checks_pending ON schedule_checks(lease_until) WHERE state='running';
GRANT SELECT,INSERT,UPDATE ON schedule_checks TO search_app;
-- +goose Down
DROP TABLE schedule_checks;
