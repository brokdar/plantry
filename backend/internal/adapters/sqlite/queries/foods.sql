-- name: CreateFood :one
INSERT INTO foods (
    name, kind, role, source, barcode, off_id, fdc_id, image_path,
    kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g,
    saturated_fat_100g, trans_fat_100g, cholesterol_100g, sugar_100g,
    potassium_100g, calcium_100g, iron_100g, magnesium_100g, phosphorus_100g, zinc_100g,
    vitamin_a_100g, vitamin_c_100g, vitamin_d_100g, vitamin_b12_100g, vitamin_b6_100g, folate_100g,
    variant_group_id, reference_portions, prep_minutes, cook_minutes, notes,
    favorite
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?
)
RETURNING *;

-- name: GetFood :one
SELECT * FROM foods WHERE id = ?;

-- name: UpdateFood :one
UPDATE foods SET
    name               = ?,
    role               = ?,
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
    variant_group_id   = ?,
    reference_portions = ?,
    prep_minutes       = ?,
    cook_minutes       = ?,
    notes              = ?,
    favorite           = ?,
    updated_at         = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteFood :execresult
DELETE FROM foods WHERE id = ?;

-- name: SetFoodFavorite :one
UPDATE foods SET
    favorite   = ?,
    updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: MarkFoodCooked :exec
UPDATE foods
SET last_cooked_at = ?,
    cook_count     = cook_count + 1,
    updated_at     = datetime('now')
WHERE id = ?;

-- name: ListForgottenFoods :many
SELECT * FROM foods
WHERE kind = 'composed' AND (last_cooked_at IS NULL OR last_cooked_at < ?)
ORDER BY (last_cooked_at IS NOT NULL), last_cooked_at ASC, name ASC
LIMIT ?;

-- name: ListMostCookedFoods :many
SELECT * FROM foods
WHERE kind = 'composed' AND cook_count > 0
ORDER BY cook_count DESC, last_cooked_at DESC, name ASC
LIMIT ?;

-- name: CreateVariantGroup :one
INSERT INTO variant_groups (name) VALUES (?) RETURNING *;

-- name: ListSiblingFoods :many
SELECT * FROM foods
WHERE kind = 'composed' AND variant_group_id = ? AND id != ?
ORDER BY name;

-- name: CreateFoodComponent :exec
INSERT INTO food_components (parent_id, child_id, amount, unit, grams, sort_order)
VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteFoodComponents :exec
DELETE FROM food_components WHERE parent_id = ?;

-- name: ListFoodComponents :many
SELECT fc.id, fc.parent_id, fc.child_id, fc.amount, fc.unit, fc.grams, fc.sort_order,
       f.name AS child_name, f.kind AS child_kind
FROM food_components fc
JOIN foods f ON f.id = fc.child_id
WHERE fc.parent_id = ?
ORDER BY fc.sort_order, fc.id;

-- name: CreateFoodInstruction :exec
INSERT INTO food_instructions (food_id, step_number, text)
VALUES (?, ?, ?);

-- name: DeleteFoodInstructions :exec
DELETE FROM food_instructions WHERE food_id = ?;

-- name: ListFoodInstructions :many
SELECT * FROM food_instructions WHERE food_id = ? ORDER BY step_number;

-- name: CreateFoodTag :exec
INSERT INTO food_tags (food_id, tag) VALUES (?, ?);

-- name: DeleteFoodTags :exec
DELETE FROM food_tags WHERE food_id = ?;

-- name: ListFoodTags :many
SELECT food_id, tag FROM food_tags WHERE food_id = ? ORDER BY tag;

-- name: ListFoodPortions :many
SELECT food_id, unit, grams
FROM food_portions
WHERE food_id = ?
ORDER BY unit;

-- name: UpsertFoodPortion :exec
INSERT INTO food_portions (food_id, unit, grams)
VALUES (?, ?, ?)
ON CONFLICT (food_id, unit) DO UPDATE SET grams = excluded.grams;

-- name: DeleteFoodPortion :execresult
DELETE FROM food_portions WHERE food_id = ? AND unit = ?;
