-- +goose Up
-- The user's "needs no cooking" override, for both kinds. NULL means derived
-- (planning.NoCook's heuristic), so nothing is backfilled.
ALTER TABLE foods ADD COLUMN no_cook INTEGER CHECK (no_cook IS NULL OR no_cook IN (0, 1));

-- +goose Down
ALTER TABLE foods DROP COLUMN no_cook;
