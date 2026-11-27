-- +goose Up
-- A "to taste" ingredient line (spec MATCH-6: salt, pepper, dried spices,
-- fresh herbs): the row keeps a nominal amount for the positive-amount checks
-- and is skipped in every nutrition sum.
ALTER TABLE food_components ADD COLUMN to_taste INTEGER NOT NULL DEFAULT 0 CHECK (to_taste IN (0, 1));

-- +goose Down
ALTER TABLE food_components DROP COLUMN to_taste;
