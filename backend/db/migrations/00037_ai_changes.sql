-- +goose Up
-- Phase 4 D13: one row per plan-writing agent tool call (a "change"), so the
-- assistant can show it as a line and undo it on its own. The change's
-- plate_events are those with ai_run_id in run_ids_json (the chat run plus a
-- regenerate run it waited for) and id in [first_event_id, last_event_id];
-- both are NULL when the call wrote no plate event (an availability change
-- on an empty cell). dates/plate ids/session ids are what the change line
-- shows and what undo reports. availability_before_json is the cell's
-- override before set_availability ({date, slot_id, override|null}); NULL
-- for the other tools.
CREATE TABLE ai_changes (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id                   INTEGER NOT NULL REFERENCES ai_runs(id) ON DELETE CASCADE,
    step_seq                 INTEGER NOT NULL,
    tool                     TEXT    NOT NULL,
    text_key                 TEXT    NOT NULL,
    params_json              TEXT    NOT NULL,
    run_ids_json             TEXT    NOT NULL,
    dates_json               TEXT    NOT NULL,
    plate_ids_json           TEXT    NOT NULL,
    session_ids_json         TEXT    NOT NULL,
    first_event_id           INTEGER,
    last_event_id            INTEGER,
    availability_before_json TEXT,
    created_at               TEXT    NOT NULL DEFAULT (datetime('now')),
    undone_at                TEXT
);
CREATE INDEX idx_ai_changes_run ON ai_changes(run_id);

-- +goose Down
DROP TABLE ai_changes;
