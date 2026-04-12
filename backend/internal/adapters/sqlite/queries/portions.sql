-- name: ListPortions :many
SELECT ingredient_id, unit, grams
FROM ingredient_portions
WHERE ingredient_id = ?
ORDER BY unit;

-- name: UpsertPortion :exec
INSERT INTO ingredient_portions (ingredient_id, unit, grams)
VALUES (?, ?, ?)
ON CONFLICT (ingredient_id, unit) DO UPDATE SET grams = excluded.grams;

-- name: DeletePortion :execresult
DELETE FROM ingredient_portions WHERE ingredient_id = ? AND unit = ?;
