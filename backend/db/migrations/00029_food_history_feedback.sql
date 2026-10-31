-- +goose Up
-- Phase 2 D16/D17: eating history is derived from plates and cook sessions,
-- so the stored cook counters go. Feedback statuses become loved, disliked
-- and not_eaten: skipped rows become not_eaten, cooked rows are deleted.
DROP INDEX ix_foods_last_cooked;
ALTER TABLE foods DROP COLUMN last_cooked_at;
ALTER TABLE foods DROP COLUMN cook_count;

CREATE TABLE plate_feedback_new (
    plate_id INTEGER PRIMARY KEY REFERENCES plates(id) ON DELETE CASCADE,
    status   TEXT NOT NULL CHECK (status IN ('loved','disliked','not_eaten')),
    note     TEXT,
    rated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO plate_feedback_new (plate_id, status, note, rated_at)
SELECT plate_id, CASE status WHEN 'skipped' THEN 'not_eaten' ELSE status END, note, rated_at
FROM plate_feedback
WHERE status <> 'cooked';
DROP INDEX ix_plate_feedback_status;
DROP TABLE plate_feedback;
ALTER TABLE plate_feedback_new RENAME TO plate_feedback;
CREATE INDEX ix_plate_feedback_status ON plate_feedback(status);

-- +goose Down
-- The deleted cooked rows and the dropped counters are not restored.
CREATE TABLE plate_feedback_old (
    plate_id INTEGER PRIMARY KEY REFERENCES plates(id) ON DELETE CASCADE,
    status   TEXT NOT NULL CHECK (status IN ('cooked','skipped','loved','disliked')),
    note     TEXT,
    rated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO plate_feedback_old (plate_id, status, note, rated_at)
SELECT plate_id, CASE status WHEN 'not_eaten' THEN 'skipped' ELSE status END, note, rated_at
FROM plate_feedback;
DROP INDEX ix_plate_feedback_status;
DROP TABLE plate_feedback;
ALTER TABLE plate_feedback_old RENAME TO plate_feedback;
CREATE INDEX ix_plate_feedback_status ON plate_feedback(status);

ALTER TABLE foods ADD COLUMN last_cooked_at TEXT;
ALTER TABLE foods ADD COLUMN cook_count INTEGER NOT NULL DEFAULT 0;
CREATE INDEX ix_foods_last_cooked ON foods(last_cooked_at);
