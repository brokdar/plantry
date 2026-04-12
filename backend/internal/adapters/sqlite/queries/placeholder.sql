-- Placeholder query so sqlc has something to generate in Phase 0. Real
-- aggregate queries replace this starting in Phase 1 (ingredients).

-- name: SchemaVersion :one
SELECT 1 AS version;
