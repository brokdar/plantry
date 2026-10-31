-- name: ListCookSessionsByCookDate :many
-- Sessions cooking in [from_date, to_date]: the recipe sources of a
-- shopping list.
SELECT * FROM cook_sessions
WHERE cook_date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date)
ORDER BY cook_date, cook_slot_id, id;

-- name: ListLeafPlateComponentsInRange :many
-- Leaf components on plates dated in [from_date, to_date] with their plate's
-- cell, eaters and state: the ingredient sources of a shopping list.
SELECT pc.food_id, pc.amount, pc.unit, pc.grams, p.date, p.slot_id, p.eaters, p.state
FROM plate_components pc
JOIN plates p ON p.id = pc.plate_id
WHERE p.date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date)
  AND pc.amount IS NOT NULL
ORDER BY p.date, p.slot_id, pc.id;
