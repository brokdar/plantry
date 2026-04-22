-- +goose Up
ALTER TABLE components ADD COLUMN favorite INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1));
ALTER TABLE plates     ADD COLUMN skipped  INTEGER NOT NULL DEFAULT 0 CHECK (skipped  IN (0, 1));

CREATE INDEX ix_components_favorite ON components(favorite) WHERE favorite = 1;

-- +goose Down
DROP INDEX IF EXISTS ix_components_favorite;
ALTER TABLE plates     DROP COLUMN skipped;
ALTER TABLE components DROP COLUMN favorite;
