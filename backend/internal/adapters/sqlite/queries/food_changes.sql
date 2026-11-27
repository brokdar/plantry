-- name: GetFoodChanges :one
SELECT changes_json FROM food_changes WHERE food_id = ?;

-- name: PutFoodChanges :exec
INSERT INTO food_changes (food_id, changes_json) VALUES (?, ?)
ON CONFLICT (food_id) DO UPDATE SET changes_json = excluded.changes_json;

-- name: DeleteFoodChanges :exec
DELETE FROM food_changes WHERE food_id = ?;

-- name: ConfirmedPlateDatesOf :many
-- The dates of confirmed plates from today on that hold the recipe (or a
-- hidden substitution variant of it): their shopping changes with an
-- in-place edit (VAR-2).
SELECT DISTINCT p.date
FROM plates p
JOIN plate_components pc ON pc.plate_id = p.id
LEFT JOIN hidden_variants hv ON hv.food_id = pc.food_id
WHERE (pc.food_id = sqlc.arg(food_id) OR hv.base_food_id = sqlc.arg(food_id))
  AND p.state = 'confirmed'
  AND p.date >= sqlc.arg(today)
ORDER BY p.date;
