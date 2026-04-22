-- +goose Up
CREATE TABLE plate_feedback (
    plate_id INTEGER PRIMARY KEY REFERENCES plates(id) ON DELETE CASCADE,
    status   TEXT NOT NULL CHECK (status IN ('cooked','skipped','loved','disliked')),
    note     TEXT,
    rated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX ix_plate_feedback_status ON plate_feedback(status);

-- +goose Down
DROP INDEX IF EXISTS ix_plate_feedback_status;
DROP TABLE IF EXISTS plate_feedback;
