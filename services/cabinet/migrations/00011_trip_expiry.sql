-- +goose Up
ALTER TABLE trip_requests DROP CONSTRAINT trip_requests_status_check;
ALTER TABLE trip_requests
    ADD CONSTRAINT trip_requests_status_check CHECK (status IN ('saved','running','cancelled','completed','expired')),
    ADD COLUMN expired_at timestamptz,
    ADD CONSTRAINT trip_expired_timestamp CHECK ((status='expired') = (expired_at IS NOT NULL));
GRANT UPDATE (expired_at) ON trip_requests TO cabinet_app;
-- The expiry loop scans only active trips by their last departure day.
CREATE INDEX trip_requests_active_departure_to ON trip_requests(departure_to) WHERE status IN ('saved','running');
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_event_type_check;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_event_type_check
    CHECK (event_type IN ('travelwatch.events.v1.TripRequestCreated','travelwatch.events.v1.TripRequestCancelled','travelwatch.events.v1.TripRequestExpired'));

-- +goose Down
-- Expired trips and their events cannot be silently discarded: restoring the old
-- checks fails while any exist, and the transaction rolls back.
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_event_type_check;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_event_type_check
    CHECK (event_type IN ('travelwatch.events.v1.TripRequestCreated','travelwatch.events.v1.TripRequestCancelled'));
DROP INDEX trip_requests_active_departure_to;
REVOKE UPDATE (expired_at) ON trip_requests FROM cabinet_app;
ALTER TABLE trip_requests DROP CONSTRAINT trip_expired_timestamp, DROP CONSTRAINT trip_requests_status_check;
ALTER TABLE trip_requests DROP COLUMN expired_at,
    ADD CONSTRAINT trip_requests_status_check CHECK (status IN ('saved','running','cancelled','completed'));
