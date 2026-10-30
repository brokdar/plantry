-- name: CreateCookSession :one
INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions, frozen_portions, cooked_at, note)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetCookSession :one
SELECT * FROM cook_sessions WHERE id = ?;

-- name: UpdateCookSession :one
UPDATE cook_sessions SET
    cook_date       = ?,
    cook_slot_id    = ?,
    portions        = ?,
    frozen_portions = ?,
    cooked_at       = ?,
    note            = ?,
    updated_at      = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteCookSession :execresult
DELETE FROM cook_sessions WHERE id = ?;

-- name: ListCookSessionLinks :many
SELECT pc.id AS component_id, pc.plate_id, pc.food_id, pc.portions, p.date, p.slot_id, p.eaters
FROM plate_components pc
JOIN plates p ON p.id = pc.plate_id
WHERE pc.cook_session_id = ?
ORDER BY p.date, p.slot_id, pc.id;
