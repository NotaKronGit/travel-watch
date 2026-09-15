-- +goose Up
CREATE TABLE outbox_events (
    id uuid PRIMARY KEY,
    request_id uuid NOT NULL REFERENCES trip_requests(id),
    event_type text NOT NULL CHECK (event_type = 'travelwatch.events.v1.TripRequestCreated'),
    payload bytea NOT NULL CHECK (octet_length(payload) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    lease_token uuid,
    last_error text NOT NULL DEFAULT '',
    UNIQUE (request_id, event_type)
);
CREATE INDEX outbox_pending_idx ON outbox_events(available_at, created_at, id) WHERE published_at IS NULL;
GRANT SELECT, INSERT, UPDATE ON outbox_events TO cabinet_app;

-- +goose Down
DROP TABLE outbox_events;
