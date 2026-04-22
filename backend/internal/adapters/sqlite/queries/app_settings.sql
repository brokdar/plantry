-- name: GetSetting :one
SELECT key, value, encrypted, updated_at
FROM app_settings
WHERE key = ?;

-- name: ListSettings :many
SELECT key, value, encrypted, updated_at
FROM app_settings
ORDER BY key;

-- name: UpsertSetting :exec
INSERT INTO app_settings (key, value, encrypted, updated_at)
VALUES (?, ?, ?, datetime('now'))
ON CONFLICT(key) DO UPDATE SET
    value      = excluded.value,
    encrypted  = excluded.encrypted,
    updated_at = datetime('now');

-- name: DeleteSetting :exec
DELETE FROM app_settings WHERE key = ?;
