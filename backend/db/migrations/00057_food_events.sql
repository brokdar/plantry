-- +goose Up
-- Audit trail of changes to recipes, attributes, taste items and
-- substitutions (VAR-5), the base of their Undo. subject_id is the changed
-- row's id (no foreign key: rows outlive their subjects); before_json and
-- after_json are snapshots, NULL when the subject did not exist (create) or
-- no longer does (delete). kind, action and actor are checked in Go.
CREATE TABLE food_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    kind        TEXT    NOT NULL,
    subject_id  INTEGER NOT NULL,
    action      TEXT    NOT NULL,
    actor       TEXT    NOT NULL,
    before_json TEXT,
    after_json  TEXT,
    at          TEXT    NOT NULL
);
CREATE INDEX idx_food_events_subject ON food_events (kind, subject_id);

-- +goose Down
DROP TABLE food_events;
