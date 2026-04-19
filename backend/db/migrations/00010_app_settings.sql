-- +goose Up
CREATE TABLE app_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    encrypted   INTEGER NOT NULL DEFAULT 0 CHECK (encrypted IN (0, 1)),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down
DROP TABLE IF EXISTS app_settings;
