-- +goose Up
ALTER TABLE catalog_cities ADD COLUMN region_name text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE catalog_cities DROP COLUMN region_name;
