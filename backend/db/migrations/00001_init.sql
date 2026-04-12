-- +goose Up
-- Baseline migration. Plantry uses WAL + foreign keys; both are set as
-- pragmas on every connection in the runtime, but we assert FKs here so that
-- any tool (goose, sqlc) touching the DB sees the same contract.
PRAGMA foreign_keys = ON;

-- +goose Down
-- No-op: there is nothing to undo for the baseline.
SELECT 1;
