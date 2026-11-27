-- name: InsertFoodOrigin :exec
INSERT INTO food_origins (food_id, kind, ref, source) VALUES (?, ?, ?, ?);

-- name: ListFoodOriginLeaves :many
SELECT food_id FROM food_origins WHERE kind = ? AND ref = ? ORDER BY food_id;
