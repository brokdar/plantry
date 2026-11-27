-- +goose Up
-- Chef CP3 fixes: passed_on is the start of the window a generation offered a
-- ready find to and did not use (it could not be added, or no meal took it).
-- Such a find stays ready for the daily idea and the user but no longer
-- counts toward the next window's New dishes (FIND-3).
ALTER TABLE recipe_suggestions ADD COLUMN passed_on TEXT;

-- +goose Down
ALTER TABLE recipe_suggestions DROP COLUMN passed_on;
