-- +goose Up
-- The light requests of the dates an apply_changes call saved, before and
-- after ({dates, before, after}), so undo puts them back. NULL when the call
-- saved no light change.
ALTER TABLE ai_changes ADD COLUMN light_before_json TEXT;

-- +goose Down
ALTER TABLE ai_changes DROP COLUMN light_before_json;
