-- +goose Up
-- Chef C4: a recipe added by the assistant starts on trial (NEW-1). trial is
-- set when it enters the library and cleared when the trial ends early
-- (archive); the natural end (eaten, two days without a bad rating, NEW-2)
-- is derived from plates (FoodHistory). trial_seen clears the To try badge
-- when the recipe is opened (NEW-3), not the trial.
ALTER TABLE foods ADD COLUMN trial INTEGER NOT NULL DEFAULT 0 CHECK (trial IN (0, 1));
ALTER TABLE foods ADD COLUMN trial_seen INTEGER NOT NULL DEFAULT 0 CHECK (trial_seen IN (0, 1));

-- +goose Down
ALTER TABLE foods DROP COLUMN trial_seen;
ALTER TABLE foods DROP COLUMN trial;
