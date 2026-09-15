-- +goose Up
CREATE INDEX trip_requests_user_created_idx ON trip_requests(user_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX trip_requests_user_created_idx;
