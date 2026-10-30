-- Schema patch: informs sqlc of the shape Go migration 00027 produces
-- (cook sessions, plates.eaters/updated_at, REAL portions with
-- plate_components.cook_session_id, foods.keep_days/freezable).
-- This file is read by sqlc only; it is NOT a goose migration.
CREATE TABLE cook_sessions (
    id              INTEGER PRIMARY KEY,
    food_id         INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    cook_date       TEXT NOT NULL,
    cook_slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
    portions        REAL NOT NULL CHECK (portions > 0),
    frozen_portions REAL NOT NULL DEFAULT 0 CHECK (frozen_portions >= 0 AND frozen_portions <= portions),
    cooked_at       TEXT,
    note            TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

ALTER TABLE plates ADD COLUMN eaters REAL NOT NULL DEFAULT 1;
ALTER TABLE plates ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';

DROP TABLE plate_components;
CREATE TABLE plate_components (
    id              INTEGER PRIMARY KEY,
    plate_id        INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    food_id         INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    portions        REAL,
    amount          REAL,
    unit            TEXT,
    grams           REAL,
    grams_source    TEXT,
    cook_session_id INTEGER REFERENCES cook_sessions(id) ON DELETE RESTRICT,
    sort_order      INTEGER NOT NULL DEFAULT 0
);

DROP TABLE preset_components;
CREATE TABLE preset_components (
    id              INTEGER PRIMARY KEY,
    preset_plate_id INTEGER NOT NULL REFERENCES preset_plates(id) ON DELETE CASCADE,
    food_id         INTEGER NOT NULL REFERENCES foods(id)         ON DELETE RESTRICT,
    portions        REAL,
    amount          REAL,
    unit            TEXT,
    grams           REAL,
    grams_source    TEXT,
    note            TEXT,
    sort_order      INTEGER NOT NULL DEFAULT 0
);

ALTER TABLE foods ADD COLUMN keep_days INTEGER;
ALTER TABLE foods ADD COLUMN freezable INTEGER;
