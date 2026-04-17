-- name: CreateConversation :one
INSERT INTO ai_conversations (week_id, title)
VALUES (?, ?)
RETURNING *;

-- name: GetConversation :one
SELECT * FROM ai_conversations WHERE id = ?;

-- name: UpdateConversationTitle :one
UPDATE ai_conversations
SET title = ?, updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: TouchConversation :execresult
UPDATE ai_conversations SET updated_at = datetime('now') WHERE id = ?;

-- name: DeleteConversation :execresult
DELETE FROM ai_conversations WHERE id = ?;

-- name: ListConversations :many
SELECT * FROM ai_conversations
ORDER BY updated_at DESC, id DESC
LIMIT ? OFFSET ?;

-- name: ListConversationsByWeek :many
SELECT * FROM ai_conversations
WHERE week_id = ?
ORDER BY updated_at DESC, id DESC
LIMIT ? OFFSET ?;

-- name: CountConversations :one
SELECT COUNT(*) FROM ai_conversations;

-- name: CountConversationsByWeek :one
SELECT COUNT(*) FROM ai_conversations WHERE week_id = ?;

-- name: AppendMessage :one
INSERT INTO ai_messages (conversation_id, role, content)
VALUES (?, ?, ?)
RETURNING *;

-- name: ListMessages :many
SELECT * FROM ai_messages
WHERE conversation_id = ?
ORDER BY id ASC;
