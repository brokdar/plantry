-- name: CreateAIChange :one
INSERT INTO ai_changes (
    run_id, step_seq, tool, text_key, params_json, run_ids_json, dates_json,
    plate_ids_json, session_ids_json, first_event_id, last_event_id, availability_before_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAIChange :one
SELECT * FROM ai_changes WHERE id = ?;

-- name: ListAIChangesByConversation :many
SELECT c.* FROM ai_changes c
JOIN ai_runs r ON r.id = c.run_id
WHERE r.conversation_id = ?
ORDER BY c.id;

-- name: MarkAIChangeUndone :execresult
UPDATE ai_changes SET undone_at = ? WHERE id = ? AND undone_at IS NULL;
