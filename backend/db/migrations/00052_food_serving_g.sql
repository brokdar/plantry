-- +goose Up
-- The grams of one serving of an eat-as-is leaf (an apple, a slice of bread):
-- the planner offers the food in this amount. NULL means none stored (the
-- planner falls back to planning.TypicalLeafGrams).
ALTER TABLE foods ADD COLUMN serving_g REAL CHECK (serving_g IS NULL OR serving_g > 0);

-- +goose Down
ALTER TABLE foods DROP COLUMN serving_g;
