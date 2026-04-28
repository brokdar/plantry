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
INSERT INTO plates (slot_id, note, date)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetPlate :one
SELECT * FROM plates WHERE id = ?;

-- name: UpdatePlate :one
UPDATE plates SET
    slot_id = ?,
    note    = ?,
    date    = ?
WHERE id = ?
RETURNING *;

-- name: SetPlateSkipped :one
UPDATE plates SET
    skipped = ?,
    note    = ?
WHERE id = ?
RETURNING *;

-- name: DeletePlate :execresult
DELETE FROM plates WHERE id = ?;

-- name: CreatePlateComponent :one
INSERT INTO plate_components (plate_id, food_id, portions, sort_order)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetPlateComponent :one
SELECT * FROM plate_components WHERE id = ?;

-- name: UpdatePlateComponent :one
UPDATE plate_components SET
    food_id  = ?,
    portions = ?
WHERE id = ?
RETURNING *;

-- name: DeletePlateComponent :execresult
DELETE FROM plate_components WHERE id = ?;

-- name: ListPlateComponentsByPlate :many
SELECT * FROM plate_components WHERE plate_id = ? ORDER BY sort_order, id;

-- name: CountPlatesUsingTimeSlot :one
SELECT COUNT(*) FROM plates WHERE slot_id = ?;

-- name: ListPlatesByDateRange :many
SELECT * FROM plates WHERE date BETWEEN ? AND ? ORDER BY date, slot_id, id;

-- name: ListPlatesByDate :many
SELECT * FROM plates WHERE date = ? ORDER BY slot_id, id;

-- name: CountPlatesUsingFood :one
SELECT COUNT(*) FROM plate_components WHERE food_id = ?;
