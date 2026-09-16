-- +goose Up
ALTER TABLE trip_requests ADD COLUMN building_stage text NOT NULL DEFAULT 'not_started';
ALTER TABLE trip_requests ADD COLUMN building_revision bigint NOT NULL DEFAULT 0;
GRANT UPDATE (building_stage, building_revision) ON trip_requests TO cabinet_app;
CREATE TABLE trip_stage_history (
 event_id uuid PRIMARY KEY,
 request_id uuid NOT NULL REFERENCES trip_requests(id),
 revision bigint NOT NULL CHECK (revision > 0),
 payload bytea NOT NULL,
 UNIQUE(request_id, revision)
);
GRANT SELECT, INSERT ON trip_stage_history TO cabinet_app;
-- +goose Down
DROP TABLE trip_stage_history;
ALTER TABLE trip_requests DROP COLUMN building_revision, DROP COLUMN building_stage;
