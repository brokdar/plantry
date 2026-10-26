-- name: ListAvailabilityDefaults :many
SELECT d.weekday, d.slot_id, d.mode, d.preset_id, p.name AS preset_name
FROM availability_defaults d
LEFT JOIN presets p ON p.id = d.preset_id
ORDER BY d.weekday, d.slot_id;

-- name: GetAvailabilityDefault :one
SELECT d.weekday, d.slot_id, d.mode, d.preset_id, p.name AS preset_name
FROM availability_defaults d
LEFT JOIN presets p ON p.id = d.preset_id
WHERE d.weekday = ? AND d.slot_id = ?;

-- name: DeleteActiveSlotAvailabilityDefaults :exec
DELETE FROM availability_defaults
WHERE slot_id IN (SELECT id FROM time_slots WHERE active = 1);

-- name: InsertAvailabilityDefault :exec
INSERT INTO availability_defaults (weekday, slot_id, mode, preset_id)
VALUES (?, ?, ?, ?);

-- name: ListAvailabilityOverridesByRange :many
SELECT o.date, o.slot_id, o.mode, o.preset_id, o.note, p.name AS preset_name
FROM availability_overrides o
LEFT JOIN presets p ON p.id = o.preset_id
WHERE o.date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date)
ORDER BY o.date, o.slot_id;

-- name: GetAvailabilityOverride :one
SELECT o.date, o.slot_id, o.mode, o.preset_id, o.note, p.name AS preset_name
FROM availability_overrides o
LEFT JOIN presets p ON p.id = o.preset_id
WHERE o.date = ? AND o.slot_id = ?;

-- name: UpsertAvailabilityOverride :exec
INSERT INTO availability_overrides (date, slot_id, mode, preset_id, note)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (date, slot_id) DO UPDATE SET
    mode       = excluded.mode,
    preset_id  = excluded.preset_id,
    note       = excluded.note,
    updated_at = datetime('now');

-- name: DeleteAvailabilityOverride :exec
DELETE FROM availability_overrides WHERE date = ? AND slot_id = ?;
