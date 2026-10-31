-- +goose Up
-- Phase 3 B4: the generator's free-text notes for a run (shown at the top of
-- the review board). NULL for runs that write none.
ALTER TABLE ai_runs ADD COLUMN notes TEXT;

-- +goose Down
ALTER TABLE ai_runs DROP COLUMN notes;
