-- +goose Up
CREATE TABLE catalog_countries (
    code text PRIMARY KEY CHECK (code ~ '^[A-Z]{2}$'),
    name text NOT NULL
);
CREATE TABLE catalog_cities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source text NOT NULL,
    source_id bigint NOT NULL,
    name text NOT NULL,
    country_code text NOT NULL REFERENCES catalog_countries(code),
    region_code text NOT NULL,
    timezone text NOT NULL,
    aliases jsonb NOT NULL,
    latitude double precision NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude double precision NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    population bigint NOT NULL CHECK (population >= 0),
    active boolean NOT NULL DEFAULT true,
    UNIQUE(source, source_id)
);
CREATE TABLE catalog_imports (
    source text PRIMARY KEY,
    version text NOT NULL,
    city_count integer NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now()
);
-- Import is an explicit administrative command using the owner role.
GRANT SELECT ON catalog_countries, catalog_cities, catalog_imports TO cabinet_app;

-- +goose Down
DROP TABLE catalog_imports;
DROP TABLE catalog_cities;
DROP TABLE catalog_countries;
