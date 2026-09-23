-- +goose Up
-- Periodic re-checks: when the next one is due, and when the last one failed while
-- the previous result was kept on screen.
ALTER TABLE schedule_checks ADD COLUMN next_check_at timestamptz, ADD COLUMN refresh_failed_at timestamptz;
-- Checks saved before this change are re-checked soon after the update.
UPDATE schedule_checks SET next_check_at=now() WHERE state IN ('done','failed');
CREATE INDEX schedule_checks_due ON schedule_checks(next_check_at) WHERE state IN ('done','failed');

-- +goose Down
DROP INDEX schedule_checks_due;
ALTER TABLE schedule_checks DROP COLUMN refresh_failed_at, DROP COLUMN next_check_at;
