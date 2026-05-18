-- +goose Up
-- +goose StatementBegin
-- The Templates feature has been fully replaced by Presets (migration 00019).
-- This migration drops the templates tables. Existing template data is not
-- migrated — per the feature spec the user accepts "burn it down" semantics.
DROP INDEX IF EXISTS ix_template_entries_template;
DROP TABLE IF EXISTS template_entries;
DROP TABLE IF EXISTS templates;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Intentional no-op: the templates feature is gone from the codebase and we
-- have no path to restore historical state. Recreate the empty tables so
-- goose can step backwards through this migration in a development DB.
CREATE TABLE IF NOT EXISTS templates (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    scope      TEXT NOT NULL DEFAULT 'slot' CHECK (scope IN ('slot','day','week'))
);
CREATE TABLE IF NOT EXISTS template_entries (
    id           INTEGER PRIMARY KEY,
    template_id  INTEGER NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    food_id      INTEGER NOT NULL REFERENCES foods(id)    ON DELETE RESTRICT,
    portions     REAL    NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0,
    day_offset   INTEGER NOT NULL DEFAULT 0,
    slot_id      INTEGER REFERENCES time_slots(id) ON DELETE RESTRICT,
    note         TEXT
);
CREATE INDEX IF NOT EXISTS ix_template_entries_template ON template_entries(template_id);
-- +goose StatementEnd
