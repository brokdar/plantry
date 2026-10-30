-- name: CreateTimeSlot :one
INSERT INTO time_slots (name_key, icon, sort_order, active)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetTimeSlot :one
SELECT * FROM time_slots WHERE id = ?;

-- name: UpdateTimeSlot :one
UPDATE time_slots SET
    name_key   = ?,
    icon       = ?,
    sort_order = ?,
    active     = ?
WHERE id = ?
RETURNING *;

-- name: DeleteTimeSlot :execresult
DELETE FROM time_slots WHERE id = ?;

-- name: ListTimeSlots :many
SELECT * FROM time_slots ORDER BY sort_order, id;

-- name: ListActiveTimeSlots :many
SELECT * FROM time_slots WHERE active = 1 ORDER BY sort_order, id;

-- name: CreatePlate :one
INSERT INTO plates (slot_id, note, date, eaters, updated_at)
VALUES (?, ?, ?, ?, datetime('now'))
RETURNING *;

-- name: GetPlate :one
SELECT * FROM plates WHERE id = ?;

-- name: UpdatePlate :one
UPDATE plates SET
    slot_id    = ?,
    note       = ?,
    date       = ?,
    eaters     = ?,
    updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: TouchPlate :exec
UPDATE plates SET updated_at = datetime('now') WHERE id = ?;

-- name: GetPlateIDByCell :one
SELECT id FROM plates WHERE date = ? AND slot_id = ?;

-- name: DeletePlate :execresult
DELETE FROM plates WHERE id = ?;

-- name: CreatePlateComponent :one
INSERT INTO plate_components (
    plate_id, food_id, portions, amount, unit, grams, grams_source, cook_session_id, sort_order
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPlateComponent :one
SELECT * FROM plate_components WHERE id = ?;

-- name: UpdatePlateComponent :one
UPDATE plate_components SET
    food_id         = ?,
    portions        = ?,
    amount          = ?,
    unit            = ?,
    grams           = ?,
    grams_source    = ?,
    cook_session_id = ?
WHERE id = ?
RETURNING *;

-- name: DeletePlateComponent :execresult
DELETE FROM plate_components WHERE id = ?;

-- name: ListPlateComponentsByPlate :many
SELECT sqlc.embed(pc), f.name AS food_name, f.kind AS food_kind, f.role AS food_role
FROM plate_components pc
JOIN foods f ON f.id = pc.food_id
WHERE pc.plate_id = ?
ORDER BY pc.sort_order, pc.id;

-- name: ListPlateComponentsByDateRange :many
SELECT sqlc.embed(pc), f.name AS food_name, f.kind AS food_kind, f.role AS food_role
FROM plate_components pc
JOIN plates p ON p.id = pc.plate_id
JOIN foods f ON f.id = pc.food_id
WHERE p.date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date)
ORDER BY pc.plate_id, pc.sort_order, pc.id;

-- name: CountPlatesUsingTimeSlot :one
SELECT COUNT(*) FROM plates WHERE slot_id = ?;

-- name: ListPlatesByDateRange :many
SELECT * FROM plates WHERE date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date) ORDER BY date, slot_id, id;

-- name: ListPlatesByDate :many
SELECT * FROM plates WHERE date = ? ORDER BY slot_id, id;

-- name: CountPlatesUsingFood :one
SELECT COUNT(*) FROM plate_components WHERE food_id = ?;

-- name: RecentUnitForFood :one
SELECT unit FROM plate_components
WHERE food_id = ? AND unit IS NOT NULL
ORDER BY id DESC
LIMIT 1;
