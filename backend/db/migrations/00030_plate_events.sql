-- +goose Up
-- Phase 2 D20: the plate audit trail. One row per plate (or, with plate_id
-- NULL, per cook session) a mutation touches, written by the plate service in
-- the mutation's transaction. No foreign keys, so rows outlive their plates,
-- sessions, slots and AI runs. The service validates action and actor (Phase
-- 3 adds values), so there are no CHECK constraints to rebuild later.
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
CREATE INDEX ix_plate_events_plate ON plate_events(plate_id);
CREATE INDEX ix_plate_events_run ON plate_events(ai_run_id);

-- +goose Down
DROP TABLE plate_events;
