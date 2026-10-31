-- name: CreatePlateEvent :exec
INSERT INTO plate_events (plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id, before_json, after_json, at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListPlateEvents :many
SELECT id, plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id, before_json, after_json, at
FROM plate_events
ORDER BY id;
