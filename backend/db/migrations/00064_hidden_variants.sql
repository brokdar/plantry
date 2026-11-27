-- +goose Up
-- Batch substitutions (HELP-7): a swap in one cook session is a hidden
-- variant of the recipe (archived, in the recipe's variant group). The row
-- marks it hidden from the archive list and names the session using it; a
-- variant the user keeps ("Save as variant") loses its row.
CREATE TABLE hidden_variants (
    food_id      INTEGER PRIMARY KEY REFERENCES foods(id) ON DELETE CASCADE,
    base_food_id INTEGER REFERENCES foods(id) ON DELETE SET NULL,
    session_id   INTEGER UNIQUE REFERENCES cook_sessions(id) ON DELETE SET NULL
);

-- +goose Down
DROP TABLE hidden_variants;
