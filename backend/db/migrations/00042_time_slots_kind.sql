-- +goose Up
-- A slot's kind tells the planner what its plates must hold: a full meal
-- needs a main, a light meal (breakfast, snack) takes anything.
ALTER TABLE time_slots ADD COLUMN kind TEXT NOT NULL DEFAULT 'meal' CHECK (kind IN ('light', 'meal'));
UPDATE time_slots SET kind = 'light' WHERE name_key IN ('slot.breakfast', 'slot.snack');

-- +goose Down
ALTER TABLE time_slots DROP COLUMN kind;
