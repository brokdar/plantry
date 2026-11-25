-- +goose Up
-- Queue of capped background work (Chef). kind is a capped ai run kind,
-- checked in Go. A job waits queued, runs, and ends done, failed or
-- skipped (switch off, no price, at the cap; the reason is in error).
-- Boot recovery puts running jobs back to queued, or fails them once
-- attempts reach the limit.
CREATE TABLE chef_jobs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    kind       TEXT    NOT NULL,
    payload    TEXT    NOT NULL DEFAULT '{}',
    requested  INTEGER NOT NULL DEFAULT 0 CHECK (requested IN (0, 1)),
    status     TEXT    NOT NULL DEFAULT 'queued'
               CHECK (status IN ('queued', 'running', 'done', 'failed', 'skipped')),
    attempts   INTEGER NOT NULL DEFAULT 0,
    error      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);
CREATE INDEX idx_chef_jobs_status ON chef_jobs (status, id);

-- +goose Down
DROP TABLE chef_jobs;
