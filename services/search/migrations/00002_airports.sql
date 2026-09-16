-- +goose Up
CREATE TABLE catalog_airports (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 source_id bigint NOT NULL UNIQUE CHECK(source_id>0),
 ident text NOT NULL,
 name text NOT NULL,
 type text NOT NULL,
 latitude double precision NOT NULL CHECK(latitude BETWEEN -90 AND 90),
 longitude double precision NOT NULL CHECK(longitude BETWEEN -180 AND 180),
 country text NOT NULL,
 region text NOT NULL,
 municipality text NOT NULL,
 iata text NOT NULL,
 icao text NOT NULL,
 scheduled boolean NOT NULL,
 active boolean NOT NULL DEFAULT true,
 imported_at timestamptz NOT NULL
);
-- IATA codes may be absent or reused; they are not identities.
CREATE INDEX catalog_airports_iata_idx ON catalog_airports(iata) WHERE active AND iata<>'';
CREATE TABLE airport_imports (
 source text PRIMARY KEY CHECK(source='ourairports'),
 row_count integer NOT NULL CHECK(row_count>0),
 fetched_at timestamptz NOT NULL,
 imported_at timestamptz NOT NULL
);
GRANT SELECT,INSERT,UPDATE ON catalog_airports,airport_imports TO search_app;
-- +goose Down
DROP TABLE airport_imports;
DROP TABLE catalog_airports;
