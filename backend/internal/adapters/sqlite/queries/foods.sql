-- name: CreateFood :one
INSERT INTO foods (
    name, kind, role, source, barcode, off_id, fdc_id, image_path,
    kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g,
    saturated_fat_100g, trans_fat_100g, cholesterol_100g, sugar_100g,
    potassium_100g, calcium_100g, iron_100g, magnesium_100g, phosphorus_100g, zinc_100g,
    vitamin_a_100g, vitamin_c_100g, vitamin_d_100g, vitamin_b12_100g, vitamin_b6_100g, folate_100g,
    variant_group_id, reference_portions, prep_minutes, cook_minutes, notes,
    favorite, source_url, description, keep_days, freezable
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetFood :one
SELECT * FROM foods WHERE id = ?;

-- name: GetLeafFoodIDByName :one
SELECT id FROM foods WHERE kind = 'leaf' AND name = ?;

-- name: GetFoodKind :one
SELECT kind FROM foods WHERE id = ?;

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
    source_url         = ?,
    description        = ?,
    keep_days          = ?,
    freezable          = ?,
    updated_at         = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteFood :execresult
DELETE FROM foods WHERE id = ?;

-- name: SetFoodArchived :one
UPDATE foods SET
    archived   = ?,
    updated_at = datetime('now')
WHERE id = ? AND kind = 'composed'
RETURNING *;

-- name: SetFoodFavorite :one
UPDATE foods SET
    favorite   = ?,
    updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: CountComposedFoods :one
SELECT COUNT(*) FROM foods WHERE kind = 'composed' AND archived = 0;

-- name: ListUneatenRecipes :many
-- Recipes no confirmed plate dated in [since, today] holds, plates marked
-- not_eaten aside: FoodHistory's last_eaten is before since, or there is
-- none. A hidden substitution variant counts as its base recipe. The
-- variety rule H8 counts them (D21). The eaten set is one
-- uncorrelated scan of the date range (ix_plates_date).
SELECT f.id, f.role
FROM foods f
WHERE f.kind = 'composed' AND f.archived = 0
  AND f.id NOT IN (
      SELECT COALESCE(hv.base_food_id, pc.food_id)
      FROM plates p
      JOIN plate_components pc ON pc.plate_id = p.id
      LEFT JOIN hidden_variants hv ON hv.food_id = pc.food_id
      WHERE p.date >= sqlc.arg(since)
        AND p.date <= sqlc.arg(today)
        AND p.state = 'confirmed'
        AND NOT EXISTS (SELECT 1 FROM plate_feedback pf
                        WHERE pf.plate_id = p.id AND pf.status = 'not_eaten'))
ORDER BY f.id;

-- name: FoodHistory :many
-- Derived eating history (D16) relative to today, over confirmed rows only
-- (D26). last_eaten is the latest confirmed plate dated on or before today
-- that holds the food and is not marked not_eaten (empty when none);
-- times_cooked counts the food's confirmed sessions cooked on or before today
-- with at least one linked plate not marked not_eaten. A hidden substitution
-- variant's plates and sessions count for its base recipe.
SELECT f.id AS food_id,
       CAST(COALESCE((
           SELECT MAX(p.date)
           FROM plate_components pc
           JOIN plates p ON p.id = pc.plate_id
           WHERE (pc.food_id = f.id OR pc.food_id IN (SELECT hv.food_id FROM hidden_variants hv WHERE hv.base_food_id = f.id))
             AND p.date <= sqlc.arg(today)
             AND p.state = 'confirmed'
             AND NOT EXISTS (SELECT 1 FROM plate_feedback pf
                             WHERE pf.plate_id = p.id AND pf.status = 'not_eaten')
       ), '') AS TEXT) AS last_eaten,
       (
           SELECT COUNT(*)
           FROM cook_sessions cs
           WHERE (cs.food_id = f.id OR cs.food_id IN (SELECT hv.food_id FROM hidden_variants hv WHERE hv.base_food_id = f.id))
             AND cs.cook_date <= sqlc.arg(today)
             AND cs.state = 'confirmed'
             AND EXISTS (SELECT 1 FROM plate_components pc
                         WHERE pc.cook_session_id = cs.id
                           AND NOT EXISTS (SELECT 1 FROM plate_feedback pf
                                           WHERE pf.plate_id = pc.plate_id AND pf.status = 'not_eaten'))
       ) AS times_cooked
FROM foods f
WHERE f.id IN (sqlc.slice(ids));

-- name: UsedInCount :many
-- The number of recipes using each leaf directly.
SELECT child_id, COUNT(DISTINCT parent_id) AS used_in
FROM food_components
WHERE child_id IN (sqlc.slice(ids))
GROUP BY child_id;

-- name: CreateVariantGroup :one
INSERT INTO variant_groups (name) VALUES (?) RETURNING *;

-- name: ListSiblingFoods :many
SELECT * FROM foods
WHERE kind = 'composed' AND archived = 0 AND variant_group_id = ? AND id != ?
ORDER BY name;

-- name: CreateFoodComponent :exec
INSERT INTO food_components (parent_id, child_id, amount, unit, grams, sort_order, to_taste)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: DeleteFoodComponents :exec
DELETE FROM food_components WHERE parent_id = ?;

-- name: ListFoodComponents :many
SELECT fc.id, fc.parent_id, fc.child_id, fc.amount, fc.unit, fc.grams, fc.sort_order, fc.to_taste,
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

-- name: ListFoodTagsByFoodIDs :many
SELECT food_id, tag FROM food_tags WHERE food_id IN (sqlc.slice(ids)) ORDER BY food_id, tag;

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

-- name: ListChildrenByParents :many
SELECT fc.id, fc.parent_id, fc.child_id, fc.amount, fc.unit, fc.grams, fc.sort_order, fc.to_taste,
       f.name AS child_name, f.kind AS child_kind
FROM food_components fc
JOIN foods f ON f.id = fc.child_id
WHERE fc.parent_id IN (sqlc.slice(parent_ids))
ORDER BY fc.parent_id, fc.sort_order, fc.id;

-- name: SetFoodDietClass :one
UPDATE foods SET
    diet_class = ?,
    updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: SetFoodNoCook :one
UPDATE foods SET
    no_cook    = ?,
    serving_g  = ?,
    updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: SetFoodShopSection :one
UPDATE foods SET
    shop_section = ?,
    updated_at   = datetime('now')
WHERE id = ?
RETURNING *;

-- name: LeafIDsUsedInRecipes :many
-- Which of the given leaves at least one active (not archived) composed
-- food uses.
SELECT DISTINCT fc.child_id
FROM food_components fc
JOIN foods p ON p.id = fc.parent_id AND p.kind = 'composed' AND p.archived = 0
WHERE fc.child_id IN (sqlc.slice(ids));

-- name: ListRecipeNames :many
-- The newest recipe names, for the discovery prompt (D8).
SELECT name FROM foods WHERE kind = 'composed' AND archived = 0 ORDER BY id DESC LIMIT ?;

-- name: ListLeafNamesByRecipeUse :many
-- Leaf names used in recipes, most used first (discovery known mode, D8).
SELECT f.name
FROM foods f
JOIN food_components fc ON fc.child_id = f.id
JOIN foods p ON p.id = fc.parent_id AND p.kind = 'composed' AND p.archived = 0
WHERE f.kind = 'leaf'
GROUP BY f.id, f.name
ORDER BY COUNT(DISTINCT fc.parent_id) DESC, f.name
LIMIT ?;

-- name: ListNeedsCheckLines :many
-- Recipe lines whose leaf lacks a core macro (legacy data, B2b). A to-taste
-- line counts no nutrition, so it is not a problem.
SELECT p.id AS recipe_id, p.name AS recipe_name,
       l.id AS leaf_id, l.name AS leaf_name,
       fc.amount, fc.unit
FROM food_components fc
JOIN foods p ON p.id = fc.parent_id AND p.kind = 'composed'
JOIN foods l ON l.id = fc.child_id AND l.kind = 'leaf'
WHERE fc.to_taste = 0
  AND p.archived = 0
  AND (l.kcal_100g IS NULL OR l.protein_100g IS NULL OR l.fat_100g IS NULL OR l.carbs_100g IS NULL)
ORDER BY p.name COLLATE NOCASE, p.id, fc.sort_order, fc.id;
