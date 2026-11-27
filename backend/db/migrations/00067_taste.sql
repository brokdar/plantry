-- +goose Up
-- +goose StatementBegin
-- Taste model (TASTE-1..7). taste_signals is append-only: one row per
-- attribute an event blamed (kind, level, key), its weight (signed evidence)
-- and share (its fractional count towards n), and what it came from.
-- taste_items holds one row per (level, key, scope): learned evidence,
-- decayed to updated_at (likes and dislikes apart, they fade at different
-- speeds), and the polarity of a confirmed statement or a user edit
-- (NULL for a learned item: its polarity follows its score). Never items
-- stay in never_items. Levels, kinds, sources and polarities are checked in
-- Go.
CREATE TABLE taste_signals (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind     TEXT    NOT NULL,
    level    TEXT    NOT NULL,
    key      TEXT    NOT NULL,
    weight   REAL    NOT NULL,
    share    REAL    NOT NULL,
    at       TEXT    NOT NULL,
    food_id  INTEGER,
    plate_id INTEGER
);
CREATE INDEX idx_taste_signals_food ON taste_signals (kind, food_id);

CREATE TABLE taste_items (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    level        TEXT    NOT NULL,
    key          TEXT    NOT NULL,
    scope        TEXT    NOT NULL DEFAULT 'always' CHECK (scope IN ('always', 'window')),
    window_start TEXT    NOT NULL DEFAULT '',
    source       TEXT    NOT NULL,
    polarity     TEXT,
    pos_evidence REAL    NOT NULL DEFAULT 0,
    pos_n        REAL    NOT NULL DEFAULT 0,
    neg_evidence REAL    NOT NULL DEFAULT 0,
    neg_n        REAL    NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL,
    UNIQUE (level, key, scope, window_start)
);
-- +goose StatementEnd

-- +goose Down
DROP TABLE taste_items;
DROP TABLE taste_signals;
