-- name: UpsertPlateFeedback :one
INSERT INTO plate_feedback (plate_id, status, note, rated_at)
VALUES (?, ?, ?, datetime('now'))
ON CONFLICT(plate_id) DO UPDATE SET
    status   = excluded.status,
    note     = excluded.note,
    rated_at = excluded.rated_at
RETURNING *;

-- name: GetPlateFeedback :one
SELECT * FROM plate_feedback WHERE plate_id = ?;

-- name: DeletePlateFeedback :execresult
DELETE FROM plate_feedback WHERE plate_id = ?;
