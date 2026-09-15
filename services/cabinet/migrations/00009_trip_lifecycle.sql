-- +goose Up
ALTER TABLE trip_requests
    ADD COLUMN status text NOT NULL DEFAULT 'saved' CHECK (status IN ('saved','running','cancelled','completed')),
    ADD COLUMN comment text NOT NULL DEFAULT '' CHECK (char_length(comment) <= 2000),
    ADD COLUMN cancelled_at timestamptz,
    ADD CONSTRAINT trip_cancelled_timestamp CHECK ((status='cancelled') = (cancelled_at IS NOT NULL));
GRANT UPDATE (status, comment, cancelled_at) ON trip_requests TO cabinet_app;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_event_type_check;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_event_type_check
    CHECK (event_type IN ('travelwatch.events.v1.TripRequestCreated','travelwatch.events.v1.TripRequestCancelled'));

-- +goose Down
-- Cancellation events cannot be silently discarded on rollback.
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_event_type_check;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_event_type_check
    CHECK (event_type = 'travelwatch.events.v1.TripRequestCreated');
REVOKE UPDATE (status, comment, cancelled_at) ON trip_requests FROM cabinet_app;
ALTER TABLE trip_requests DROP CONSTRAINT trip_cancelled_timestamp,
    DROP COLUMN cancelled_at, DROP COLUMN comment, DROP COLUMN status;
