-- name: CreatePreset :one
INSERT INTO presets (name) VALUES (?) RETURNING *;

-- name: GetPreset :one
SELECT * FROM presets WHERE id = ?;

-- name: UpdatePresetName :one
UPDATE presets SET name = ?, updated_at = datetime('now') WHERE id = ? RETURNING *;

-- name: TouchPresetUpdatedAt :exec
UPDATE presets SET updated_at = datetime('now') WHERE id = ?;

-- name: TouchPresetLastUsedAt :exec
UPDATE presets SET last_used_at = datetime('now'), updated_at = datetime('now') WHERE id = ?;

-- name: DeletePreset :execresult
DELETE FROM presets WHERE id = ?;

-- name: ListPresets :many
SELECT * FROM presets ORDER BY name COLLATE NOCASE, id;

-- name: ListPresetsByLastUsed :many
SELECT * FROM presets ORDER BY (last_used_at IS NULL), last_used_at DESC, name COLLATE NOCASE, id;

-- name: CreatePresetPlate :one
INSERT INTO preset_plates (preset_id, slot_id, sort_order) VALUES (?, ?, ?) RETURNING *;

-- name: ListPresetPlatesByPreset :many
SELECT * FROM preset_plates WHERE preset_id = ? ORDER BY sort_order, id;

-- name: DeletePresetPlatesByPreset :execresult
DELETE FROM preset_plates WHERE preset_id = ?;

-- name: CreatePresetComponent :one
INSERT INTO preset_components (
    preset_plate_id, food_id, portions, amount, unit, grams, grams_source, note, sort_order
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListPresetComponentsByPlate :many
SELECT * FROM preset_components WHERE preset_plate_id = ? ORDER BY sort_order, id;

-- name: ListPresetComponentsByPreset :many
SELECT pc.* FROM preset_components pc
JOIN preset_plates pp ON pp.id = pc.preset_plate_id
WHERE pp.preset_id = ?
ORDER BY pp.sort_order, pp.id, pc.sort_order, pc.id;

-- name: AddPresetTag :exec
INSERT OR IGNORE INTO preset_tags (preset_id, tag) VALUES (?, ?);

-- name: DeletePresetTag :exec
DELETE FROM preset_tags WHERE preset_id = ? AND tag = ?;

-- name: DeletePresetTagsByPreset :exec
DELETE FROM preset_tags WHERE preset_id = ?;

-- name: ListPresetTagsByPreset :many
SELECT tag FROM preset_tags WHERE preset_id = ? ORDER BY tag;

-- name: ListPresetTagsByPresets :many
SELECT preset_id, tag FROM preset_tags WHERE preset_id IN (sqlc.slice('preset_ids'));

-- name: ListKnownTags :many
SELECT tag, COUNT(*) AS usage_count
FROM preset_tags
GROUP BY tag
ORDER BY usage_count DESC, tag ASC
LIMIT ?;

-- name: CountPresetsUsingFood :one
SELECT COUNT(*) FROM preset_components WHERE food_id = ?;

-- name: ListPresetIDsBySlotID :many
SELECT DISTINCT preset_id FROM preset_plates WHERE slot_id = ?;

-- name: ListPresetIDsByTag :many
SELECT preset_id FROM preset_tags WHERE tag = ?;
