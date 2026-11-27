-- +goose Up
-- Origin of a catalogue leaf the matching path added for a recipe (spec
-- MATCH-3, "Catalogue leaves: origin"): import or find, the recipe it was
-- added for (its source URL or find id) and the external source id.
CREATE TABLE food_origins (
    food_id    INTEGER PRIMARY KEY REFERENCES foods(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('import', 'find')),
    ref        TEXT NOT NULL,
    source     TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX idx_food_origins_ref ON food_origins (kind, ref);

-- +goose Down
DROP TABLE food_origins;
