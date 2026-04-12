-- +goose Up
CREATE TABLE ingredient_portions (
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    unit          TEXT NOT NULL,
    grams         REAL NOT NULL CHECK (grams > 0),
    PRIMARY KEY (ingredient_id, unit)
);

-- +goose Down
DROP TABLE IF EXISTS ingredient_portions;
