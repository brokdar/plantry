-- +goose Up
CREATE TABLE weeks (
    id          INTEGER PRIMARY KEY,
    year        INTEGER NOT NULL,
    week_number INTEGER NOT NULL CHECK (week_number BETWEEN 1 AND 53),
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (year, week_number)
);

CREATE TABLE time_slots (
    id         INTEGER PRIMARY KEY,
    name_key   TEXT NOT NULL,
    icon       TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    active     INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE plates (
    id         INTEGER PRIMARY KEY,
    week_id    INTEGER NOT NULL REFERENCES weeks(id) ON DELETE CASCADE,
    day        INTEGER NOT NULL CHECK (day BETWEEN 0 AND 6),
    slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
    note       TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX ix_plates_week_day_slot ON plates(week_id, day, slot_id);

CREATE TABLE plate_components (
    id           INTEGER PRIMARY KEY,
    plate_id     INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    component_id INTEGER NOT NULL REFERENCES components(id) ON DELETE RESTRICT,
    portions     REAL NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX ix_plate_components_plate ON plate_components(plate_id);

-- +goose Down
DROP INDEX IF EXISTS ix_plate_components_plate;
DROP TABLE IF EXISTS plate_components;
DROP INDEX IF EXISTS ix_plates_week_day_slot;
DROP TABLE IF EXISTS plates;
DROP TABLE IF EXISTS time_slots;
DROP TABLE IF EXISTS weeks;
