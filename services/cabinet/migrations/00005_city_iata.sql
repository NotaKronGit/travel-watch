-- +goose Up
ALTER TABLE catalog_cities ADD COLUMN iata_code text NOT NULL DEFAULT '' CHECK (iata_code = '' OR iata_code ~ '^[A-Z]{3}$');

-- +goose Down
ALTER TABLE catalog_cities DROP COLUMN iata_code;
