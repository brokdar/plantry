-- name: CreateWeek :one
INSERT INTO weeks (year, week_number) VALUES (?, ?) RETURNING *;

-- name: GetWeek :one
SELECT * FROM weeks WHERE id = ?;

-- name: GetWeekByYearAndNumber :one
SELECT * FROM weeks WHERE year = ? AND week_number = ?;

-- name: ListWeeks :many
SELECT * FROM weeks ORDER BY year DESC, week_number DESC LIMIT ? OFFSET ?;

-- name: CountWeeks :one
SELECT COUNT(*) FROM weeks;

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
INSERT INTO plates (week_id, day, slot_id, note)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetPlate :one
SELECT * FROM plates WHERE id = ?;

-- name: UpdatePlate :one
UPDATE plates SET
    day     = ?,
    slot_id = ?,
    note    = ?
WHERE id = ?
RETURNING *;

-- name: SetPlateSkipped :one
UPDATE plates SET
    skipped = ?,
    note    = ?
WHERE id = ?
RETURNING *;

-- name: DeletePlatesByWeek :execresult
DELETE FROM plates WHERE week_id = ?;

-- name: DeletePlate :execresult
DELETE FROM plates WHERE id = ?;

-- name: ListPlatesByWeek :many
SELECT * FROM plates WHERE week_id = ? ORDER BY day, slot_id, id;

-- name: CreatePlateComponent :one
INSERT INTO plate_components (plate_id, component_id, portions, sort_order)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetPlateComponent :one
SELECT * FROM plate_components WHERE id = ?;

-- name: UpdatePlateComponent :one
UPDATE plate_components SET
    component_id = ?,
    portions     = ?
WHERE id = ?
RETURNING *;

-- name: DeletePlateComponent :execresult
DELETE FROM plate_components WHERE id = ?;

-- name: ListPlateComponentsByPlate :many
SELECT * FROM plate_components WHERE plate_id = ? ORDER BY sort_order, id;

-- name: ListPlateComponentsByWeek :many
SELECT pc.*
FROM plate_components pc
JOIN plates p ON p.id = pc.plate_id
WHERE p.week_id = ?
ORDER BY p.day, p.slot_id, pc.sort_order, pc.id;

-- name: CountPlatesUsingTimeSlot :one
SELECT COUNT(*) FROM plates WHERE slot_id = ?;

-- name: CountPlatesUsingComponent :one
SELECT COUNT(*) FROM plate_components WHERE component_id = ?;
