-- +goose Up
ALTER TABLE catalog_cities ADD COLUMN name_ru text NOT NULL DEFAULT '';
ALTER TABLE catalog_countries ADD COLUMN name_ru text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE catalog_countries DROP COLUMN name_ru;
ALTER TABLE catalog_cities DROP COLUMN name_ru;
