-- +goose Up
-- Expiry stops further work like cancellation, but keeps saved results readable.
ALTER TABLE search_requests DROP CONSTRAINT search_requests_status_check;
ALTER TABLE search_requests
    ADD CONSTRAINT search_requests_status_check CHECK (status IN ('pending','cancelled','expired')),
    ADD COLUMN expired_at timestamptz,
    ADD CONSTRAINT search_requests_expired_timestamp CHECK ((status='expired') = (expired_at IS NOT NULL));

-- +goose Down
-- Fails while expired requests exist; the transaction rolls back rather than lose them.
ALTER TABLE search_requests DROP CONSTRAINT search_requests_expired_timestamp, DROP CONSTRAINT search_requests_status_check;
ALTER TABLE search_requests DROP COLUMN expired_at,
    ADD CONSTRAINT search_requests_status_check CHECK (status IN ('pending','cancelled'));
