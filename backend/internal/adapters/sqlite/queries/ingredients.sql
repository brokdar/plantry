-- name: CreateIngredient :one
INSERT INTO ingredients (
    name, source, barcode, off_id, fdc_id, image_path,
    kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetIngredient :one
SELECT * FROM ingredients WHERE id = ?;

-- name: UpdateIngredient :one
UPDATE ingredients SET
    name         = ?,
    source       = ?,
    barcode      = ?,
    off_id       = ?,
    fdc_id       = ?,
    image_path   = ?,
    kcal_100g    = ?,
    protein_100g = ?,
    fat_100g     = ?,
    carbs_100g   = ?,
    fiber_100g   = ?,
    sodium_100g  = ?,
    updated_at   = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteIngredient :execresult
DELETE FROM ingredients WHERE id = ?;
