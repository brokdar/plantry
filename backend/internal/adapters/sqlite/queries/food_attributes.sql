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
