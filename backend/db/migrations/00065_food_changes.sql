-- +goose Up
-- The changes list of a recipe the assistant changed (VAR-1, VAR-2, spec §7
-- "changes list"): what changed and why, one sentence per entry, as a JSON
-- array of strings. A filled variant gets its list on creation; an in-place
-- edit replaces the recipe's list.
CREATE TABLE food_changes (
    food_id      INTEGER PRIMARY KEY REFERENCES foods(id) ON DELETE CASCADE,
    changes_json TEXT    NOT NULL
);

-- +goose Down
DROP TABLE food_changes;
