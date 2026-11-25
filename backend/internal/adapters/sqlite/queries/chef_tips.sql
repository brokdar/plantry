-- name: CreateChefTip :one
INSERT INTO chef_tips (plate_id, recipe_id, window_start, kind, dimension, change, state, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 'shown', ?, ?)
RETURNING id;

-- name: GetChefTip :one
SELECT id, plate_id, recipe_id, window_start, kind, dimension, change, state, created_at, updated_at
FROM chef_tips WHERE id = ?;

-- name: ListChefTipsByWindow :many
SELECT id, plate_id, recipe_id, window_start, kind, dimension, change, state, created_at, updated_at
FROM chef_tips WHERE window_start = ? ORDER BY id;

-- name: SetChefTipState :exec
UPDATE chef_tips SET state = ?, updated_at = ? WHERE id = ?;

-- name: ListChefTipsDismissedSince :many
SELECT kind, updated_at FROM chef_tips WHERE state = 'dismissed' AND updated_at >= ? ORDER BY updated_at;

-- name: CountChefTipsCreatedBetween :many
SELECT state, COUNT(*) AS n FROM chef_tips WHERE created_at >= ? AND created_at < ? GROUP BY state;
