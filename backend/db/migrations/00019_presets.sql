-- +goose Up
-- +goose StatementBegin
-- Presets are the unified replacement for the old three-scoped Template system.
-- A Preset is a named, tagged, calendar-agnostic bundle of one or more plates,
-- each bound to a slot (FK to time_slots). Plates carry kind-aware components
-- mirroring the planner's plate_components shape from day one (no legacy
-- float-portions intermediary). See feature.md §4.
--
-- The old templates table is left in place for now; cutover migration 00020
-- drops it in a later phase. This migration is purely additive.

CREATE TABLE presets (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT
);

CREATE TABLE preset_plates (
    id         INTEGER PRIMARY KEY,
    preset_id  INTEGER NOT NULL REFERENCES presets(id)    ON DELETE CASCADE,
    slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_preset_plates_preset ON preset_plates(preset_id);

CREATE TABLE preset_components (
    id              INTEGER PRIMARY KEY,
    preset_plate_id INTEGER NOT NULL REFERENCES preset_plates(id) ON DELETE CASCADE,
    food_id         INTEGER NOT NULL REFERENCES foods(id)         ON DELETE RESTRICT,
    portions        INTEGER,
    amount          REAL,
    unit            TEXT,
    grams           REAL,
    grams_source    TEXT,
    note            TEXT,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    -- Mirror the plate_components CHECK constraint (added in 00018): exactly
    -- one of (portions) or (amount + unit + grams) per row.
    CHECK (
        (portions IS NOT NULL AND portions > 0
            AND amount IS NULL AND unit IS NULL AND grams IS NULL AND grams_source IS NULL)
        OR
        (portions IS NULL
            AND amount IS NOT NULL AND amount > 0
            AND unit IS NOT NULL AND unit != ''
            AND grams IS NOT NULL AND grams >= 0)
    )
);
CREATE INDEX ix_preset_components_plate ON preset_components(preset_plate_id);

CREATE TABLE preset_tags (
    preset_id INTEGER NOT NULL REFERENCES presets(id) ON DELETE CASCADE,
    tag       TEXT NOT NULL,
    PRIMARY KEY (preset_id, tag)
);

-- FTS5 index over preset names. Tag filtering joins preset_tags directly.
-- Search input is sanitised through sanitizeFTS5() in the adapter layer.
CREATE VIRTUAL TABLE presets_fts USING fts5(name, content='presets', content_rowid='id');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER presets_ai AFTER INSERT ON presets BEGIN
    INSERT INTO presets_fts(rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER presets_ad AFTER DELETE ON presets BEGIN
    INSERT INTO presets_fts(presets_fts, rowid, name) VALUES('delete', old.id, old.name);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER presets_au AFTER UPDATE ON presets BEGIN
    INSERT INTO presets_fts(presets_fts, rowid, name) VALUES('delete', old.id, old.name);
    INSERT INTO presets_fts(rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS presets_au;
DROP TRIGGER IF EXISTS presets_ad;
DROP TRIGGER IF EXISTS presets_ai;
DROP TABLE IF EXISTS presets_fts;
DROP TABLE IF EXISTS preset_tags;
DROP INDEX IF EXISTS ix_preset_components_plate;
DROP TABLE IF EXISTS preset_components;
DROP INDEX IF EXISTS ix_preset_plates_preset;
DROP TABLE IF EXISTS preset_plates;
DROP TABLE IF EXISTS presets;
-- +goose StatementEnd
