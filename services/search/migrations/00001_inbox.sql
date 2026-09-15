-- +goose Up
CREATE TABLE search_requests (
    request_id uuid PRIMARY KEY,
    status text NOT NULL CHECK (status IN ('pending', 'cancelled')),
    created_payload bytea,
    created_at timestamptz,
    cancelled_at timestamptz,
    received_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((created_payload IS NULL) = (created_at IS NULL)),
    CHECK (status <> 'pending' OR created_payload IS NOT NULL),
    CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL))
);
-- The original protobuf snapshot contains the cities, dates and passengers.
-- It remains immutable; searchable projections belong to the planning stage.
CREATE TABLE inbox_events (
    event_id uuid PRIMARY KEY,
    request_id uuid NOT NULL,
    event_type text NOT NULL,
    payload_hash bytea NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE ON search_requests TO search_app;
GRANT SELECT, INSERT ON inbox_events TO search_app;

-- +goose Down
DROP TABLE inbox_events;
DROP TABLE search_requests;
