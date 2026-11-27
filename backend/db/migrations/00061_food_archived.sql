-- +goose Up
-- Archive (NEW-4, LIB-4): a recipe hidden from planning and lists. Plates,
-- history and cook mode still reach it by id.
ALTER TABLE foods ADD COLUMN archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1));

-- +goose Down
ALTER TABLE foods DROP COLUMN archived;
