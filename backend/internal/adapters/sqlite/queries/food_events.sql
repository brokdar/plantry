-- name: CreateFoodEvent :exec
INSERT INTO food_events (kind, subject_id, action, actor, before_json, after_json, at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: LastFoodEventID :one
SELECT CAST(COALESCE(MAX(id), 0) AS INTEGER) FROM food_events;

-- name: ListFoodEventsInRange :many
SELECT id, kind, subject_id, action, actor, before_json, after_json, at
FROM food_events
WHERE id >= sqlc.arg(first_id) AND id <= sqlc.arg(last_id)
ORDER BY id;
