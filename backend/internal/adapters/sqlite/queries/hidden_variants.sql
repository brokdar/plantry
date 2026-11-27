-- name: GetHiddenVariant :one
SELECT food_id, base_food_id, session_id FROM hidden_variants WHERE food_id = ?;

-- name: UpsertHiddenVariant :exec
INSERT INTO hidden_variants (food_id, base_food_id, session_id) VALUES (?, ?, ?)
ON CONFLICT (food_id) DO UPDATE SET base_food_id = excluded.base_food_id, session_id = excluded.session_id;

-- name: ClearHiddenVariantSession :exec
-- Frees the session's slot for another variant.
UPDATE hidden_variants SET session_id = NULL WHERE session_id = ?;

-- name: ListUnusedHiddenVariantIDs :many
-- Hidden variants no cook session or plate component points at.
SELECT hv.food_id FROM hidden_variants hv
WHERE NOT EXISTS (SELECT 1 FROM cook_sessions cs WHERE cs.food_id = hv.food_id)
  AND NOT EXISTS (SELECT 1 FROM plate_components pc WHERE pc.food_id = hv.food_id);

-- name: SetSessionFood :exec
UPDATE cook_sessions SET food_id = ?, updated_at = datetime('now') WHERE id = ?;

-- name: SetSessionComponentsFood :exec
UPDATE plate_components SET food_id = ? WHERE cook_session_id = ?;

-- name: TouchPlatesOfSession :exec
UPDATE plates SET updated_at = datetime('now')
WHERE id IN (SELECT plate_id FROM plate_components WHERE cook_session_id = ?);

-- name: ListHiddenVariantBases :many
-- The recipe each of the foods is a hidden variant of.
SELECT food_id, base_food_id FROM hidden_variants
WHERE food_id IN (sqlc.slice(ids)) AND base_food_id IS NOT NULL;
