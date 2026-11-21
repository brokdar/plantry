-- +goose Up
-- The number a user names for a day (B-1, P-3) becomes that day's target: a
-- day request with a target keeps it here; 0 is a day request without one.
ALTER TABLE plan_light ADD COLUMN target_kcal REAL NOT NULL DEFAULT 0 CHECK (target_kcal >= 0);

-- +goose Down
ALTER TABLE plan_light DROP COLUMN target_kcal;
