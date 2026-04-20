-- name: CreateIngredient :one
INSERT INTO ingredients (
    name, source, barcode, off_id, fdc_id, image_path,
    kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g,
    saturated_fat_100g, trans_fat_100g, cholesterol_100g, sugar_100g,
    potassium_100g, calcium_100g, iron_100g, magnesium_100g, phosphorus_100g, zinc_100g,
    vitamin_a_100g, vitamin_c_100g, vitamin_d_100g, vitamin_b12_100g, vitamin_b6_100g, folate_100g
) VALUES (
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetIngredient :one
SELECT * FROM ingredients WHERE id = ?;

-- name: UpdateIngredient :one
UPDATE ingredients SET
    name               = ?,
    source             = ?,
    barcode            = ?,
    off_id             = ?,
    fdc_id             = ?,
    image_path         = ?,
    kcal_100g          = ?,
    protein_100g       = ?,
    fat_100g           = ?,
    carbs_100g         = ?,
    fiber_100g         = ?,
    sodium_100g        = ?,
    saturated_fat_100g = ?,
    trans_fat_100g     = ?,
    cholesterol_100g   = ?,
    sugar_100g         = ?,
    potassium_100g     = ?,
    calcium_100g       = ?,
    iron_100g          = ?,
    magnesium_100g     = ?,
    phosphorus_100g    = ?,
    zinc_100g          = ?,
    vitamin_a_100g     = ?,
    vitamin_c_100g     = ?,
    vitamin_d_100g     = ?,
    vitamin_b12_100g   = ?,
    vitamin_b6_100g    = ?,
    folate_100g        = ?,
    updated_at         = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteIngredient :execresult
DELETE FROM ingredients WHERE id = ?;
