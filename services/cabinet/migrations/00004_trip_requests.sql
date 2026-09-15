-- +goose Up
CREATE INDEX catalog_cities_name_ru_prefix_idx ON catalog_cities (lower(name_ru) text_pattern_ops) WHERE active;
CREATE TABLE trip_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    request_id uuid NOT NULL,
    origin_id uuid NOT NULL REFERENCES catalog_cities(id),
    destination_id uuid NOT NULL REFERENCES catalog_cities(id),
    departure_from date NOT NULL,
    departure_to date NOT NULL,
    adults integer NOT NULL CHECK (adults BETWEEN 1 AND 9),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (origin_id <> destination_id),
    CHECK (departure_to >= departure_from),
    UNIQUE(user_id, request_id)
);
GRANT SELECT, INSERT ON trip_requests TO cabinet_app;
-- +goose Down
DROP TABLE trip_requests;
DROP INDEX catalog_cities_name_ru_prefix_idx;
