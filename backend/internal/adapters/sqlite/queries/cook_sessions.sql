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

-- name: ListCookSessionsInRange :many
-- Sessions cooking in [from_date, to_date] or linked to a plate dated in it.
SELECT * FROM cook_sessions
WHERE cook_date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date)
   OR id IN (
       SELECT pc.cook_session_id FROM plate_components pc
       JOIN plates p ON p.id = pc.plate_id
       WHERE p.date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date)
         AND pc.cook_session_id IS NOT NULL)
ORDER BY cook_date, cook_slot_id, id;

-- name: ListCookSessionLinksInRange :many
-- Every link of the sessions ListCookSessionsInRange returns.
SELECT pc.cook_session_id, pc.id AS component_id, pc.plate_id, pc.food_id, pc.portions, p.date, p.slot_id, p.eaters
FROM plate_components pc
JOIN plates p ON p.id = pc.plate_id
WHERE pc.cook_session_id IN (
    SELECT cs.id FROM cook_sessions cs
    WHERE cs.cook_date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date)
       OR cs.id IN (
           SELECT pc2.cook_session_id FROM plate_components pc2
           JOIN plates p2 ON p2.id = pc2.plate_id
           WHERE p2.date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date)
             AND pc2.cook_session_id IS NOT NULL))
ORDER BY pc.cook_session_id, p.date, p.slot_id, pc.id;
