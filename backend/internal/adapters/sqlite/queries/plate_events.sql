-- name: CreatePlateEvent :exec
INSERT INTO plate_events (plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id, before_json, after_json, at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListPlateEvents :many
SELECT id, plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id, before_json, after_json, at
FROM plate_events
ORDER BY id;

-- name: ListUserPlateUpdates :many
-- Scans plate_events without a date/actor index: fine for a single-user audit
-- table. Newest first so the cap drops the oldest rows.
SELECT date, slot_id, before_json, after_json
FROM plate_events
WHERE actor = 'user' AND action = 'update' AND plate_id IS NOT NULL
  AND date >= ? AND date < ?
  AND before_json IS NOT NULL AND after_json IS NOT NULL
ORDER BY id DESC
LIMIT 500;

-- name: LastPlateEventID :one
SELECT CAST(COALESCE(MAX(id), 0) AS INTEGER) FROM plate_events;

-- name: ListPlateEventsOfRuns :many
SELECT id, plate_id, cook_session_id, date, slot_id, action, actor, ai_run_id, before_json, after_json, at
FROM plate_events
WHERE id > sqlc.arg(after_id) AND id <= sqlc.arg(through_id) AND ai_run_id IN (sqlc.slice(run_ids))
ORDER BY id;

-- name: CountPlateEventsAfter :one
SELECT COUNT(*) FROM plate_events
WHERE id > sqlc.arg(after_id)
  AND (plate_id IN (sqlc.slice(plate_ids)) OR cook_session_id IN (sqlc.slice(session_ids)));
