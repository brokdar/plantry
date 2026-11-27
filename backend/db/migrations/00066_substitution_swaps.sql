-- +goose Up
-- How often the same swap (recipe, from, to) was applied in a cook session
-- (HELP-7): the second time the dialog offers once to make it permanent.
CREATE TABLE substitution_swaps (
    base_food_id INTEGER NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    from_food_id INTEGER NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    to_food_id   INTEGER NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    swaps        INTEGER NOT NULL DEFAULT 1,
    offered      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (base_food_id, from_food_id, to_food_id)
);

-- +goose Down
DROP TABLE substitution_swaps;
