-- name: GetFoodAttributes :one
SELECT food_id, version, values_json, sources_json FROM food_attributes WHERE food_id = ?;

-- name: ListFoodAttributes :many
SELECT food_id, version, values_json, sources_json FROM food_attributes WHERE food_id IN (sqlc.slice(ids));

-- name: UpsertFoodAttributes :exec
INSERT INTO food_attributes (food_id, version, values_json, sources_json)
VALUES (?, ?, ?, ?)
ON CONFLICT (food_id) DO UPDATE SET
    version = excluded.version,
    values_json = excluded.values_json,
    sources_json = excluded.sources_json;

-- name: DeleteFoodAttributes :exec
DELETE FROM food_attributes WHERE food_id = ?;

-- name: ListUnlabelledRecipeIDs :many
-- Composed foods with at least one ingredient whose stored attributes hold
-- no assistant (auto) value: what the library labelling pass labels.
SELECT f.id FROM foods f
WHERE f.kind = 'composed' AND f.archived = 0
  AND EXISTS (SELECT 1 FROM food_components c WHERE c.parent_id = f.id)
  AND NOT EXISTS (
      SELECT 1 FROM food_attributes a
      WHERE a.food_id = f.id AND a.sources_json LIKE '%"auto"%')
ORDER BY f.id;
