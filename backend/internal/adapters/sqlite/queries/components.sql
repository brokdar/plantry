-- name: CreateComponent :one
INSERT INTO components (name, role, variant_group_id, reference_portions, prep_minutes, cook_minutes, image_path, notes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetComponent :one
SELECT * FROM components WHERE id = ?;

-- name: UpdateComponent :one
UPDATE components SET
    name = ?,
    role = ?,
    variant_group_id = ?,
    reference_portions = ?,
    prep_minutes = ?,
    cook_minutes = ?,
    image_path = ?,
    notes = ?,
    updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: SetComponentFavorite :one
UPDATE components SET
    favorite   = ?,
    updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: ListFavoriteComponents :many
SELECT * FROM components WHERE favorite = 1 ORDER BY name;

-- name: DeleteComponent :execresult
DELETE FROM components WHERE id = ?;

-- name: CreateComponentIngredient :exec
INSERT INTO component_ingredients (component_id, ingredient_id, amount, unit, grams, sort_order)
VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteComponentIngredients :exec
DELETE FROM component_ingredients WHERE component_id = ?;

-- name: ListComponentIngredients :many
SELECT * FROM component_ingredients WHERE component_id = ? ORDER BY sort_order;

-- name: CreateComponentInstruction :exec
INSERT INTO component_instructions (component_id, step_number, text)
VALUES (?, ?, ?);

-- name: DeleteComponentInstructions :exec
DELETE FROM component_instructions WHERE component_id = ?;

-- name: ListComponentInstructions :many
SELECT * FROM component_instructions WHERE component_id = ? ORDER BY step_number;

-- name: CreateComponentTag :exec
INSERT INTO component_tags (component_id, tag) VALUES (?, ?);

-- name: DeleteComponentTags :exec
DELETE FROM component_tags WHERE component_id = ?;

-- name: ListComponentTags :many
SELECT component_id, tag FROM component_tags WHERE component_id = ? ORDER BY tag;

-- name: CreateVariantGroup :one
INSERT INTO variant_groups (name) VALUES (?) RETURNING *;

-- name: ListSiblingComponents :many
SELECT * FROM components WHERE variant_group_id = ? AND id != ? ORDER BY name;

-- name: MarkComponentCooked :exec
UPDATE components
SET last_cooked_at = ?,
    cook_count     = cook_count + 1,
    updated_at     = datetime('now')
WHERE id = ?;

-- name: ListForgottenComponents :many
SELECT * FROM components
WHERE last_cooked_at IS NULL OR last_cooked_at < ?
ORDER BY (last_cooked_at IS NOT NULL), last_cooked_at ASC, name ASC
LIMIT ?;

-- name: ListMostCookedComponents :many
SELECT * FROM components
WHERE cook_count > 0
ORDER BY cook_count DESC, last_cooked_at DESC, name ASC
LIMIT ?;
