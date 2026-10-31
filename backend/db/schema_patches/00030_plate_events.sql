-- Schema patch: informs sqlc of the plate_events table created in Go
-- migration 00030 (which also rebuilds plates and cook_sessions with
-- AUTOINCREMENT; their columns are unchanged).
-- This file is read by sqlc only; it is NOT a goose migration.
CREATE TABLE plate_events (
    id              INTEGER PRIMARY KEY,
    plate_id        INTEGER,
    cook_session_id INTEGER,
    date            TEXT    NOT NULL,
    slot_id         INTEGER NOT NULL,
    action          TEXT    NOT NULL,
    actor           TEXT    NOT NULL,
    ai_run_id       INTEGER,
    before_json     TEXT,
    after_json      TEXT,
    at              TEXT    NOT NULL DEFAULT (datetime('now'))
);
