-- name: CreatePlateEvent :exec
INSERT INTO plate_events (plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id, before_json, after_json, at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListPlateEvents :many
SELECT id, plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id, before_json, after_json, at
FROM plate_events
ORDER BY id;

-- name: ListUserPlateUpdates :many
SELECT date, slot_id, before_json, after_json
FROM plate_events
WHERE actor = 'user' AND action = 'update' AND plate_id IS NOT NULL
  AND date >= ? AND date < ?
  AND before_json IS NOT NULL AND after_json IS NOT NULL
ORDER BY id
LIMIT 500;
