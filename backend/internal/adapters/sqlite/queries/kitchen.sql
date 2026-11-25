-- name: ListBasicsAtHome :many
SELECT f.id, f.name FROM basics_at_home b JOIN foods f ON f.id = b.food_id WHERE b.removed = 0 ORDER BY f.name;

-- name: AddBasicAtHome :exec
INSERT INTO basics_at_home (food_id) VALUES (?) ON CONFLICT (food_id) DO UPDATE SET removed = 0;

-- name: MatchBasicAtHome :exec
INSERT INTO basics_at_home (food_id) VALUES (?) ON CONFLICT DO NOTHING;

-- name: RemoveBasicAtHome :exec
UPDATE basics_at_home SET removed = 1 WHERE food_id = ?;

-- name: ListNeverItems :many
SELECT f.id, f.name FROM never_items n JOIN foods f ON f.id = n.food_id ORDER BY f.name;

-- name: AddNeverItem :exec
INSERT INTO never_items (food_id) VALUES (?) ON CONFLICT DO NOTHING;

-- name: RemoveNeverItem :exec
DELETE FROM never_items WHERE food_id = ?;
