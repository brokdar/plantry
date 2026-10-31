-- +goose Up
-- Phase 3 D28: a food's diet class override, for leaves and recipes. NULL
-- means the class is derived (diet.Classify for a leaf, the max of the
-- children for a recipe), so nothing is backfilled.
ALTER TABLE foods ADD COLUMN diet_class TEXT
    CHECK (diet_class IS NULL OR diet_class IN ('vegan','vegetarian','pescetarian','omnivore'));

-- +goose Down
ALTER TABLE foods DROP COLUMN diet_class;
