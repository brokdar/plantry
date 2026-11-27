-- name: CreateTasteSignal :exec
INSERT INTO taste_signals (kind, level, key, weight, share, at, food_id, plate_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: HasTasteSignal :one
SELECT CAST(EXISTS (SELECT 1 FROM taste_signals WHERE kind = ? AND food_id = ?) AS INTEGER);

-- name: ListTasteItems :many
SELECT id, level, key, scope, window_start, source, polarity, pos_evidence, pos_n, neg_evidence, neg_n, created_at, updated_at
FROM taste_items ORDER BY id;

-- name: GetTasteItem :one
SELECT id, level, key, scope, window_start, source, polarity, pos_evidence, pos_n, neg_evidence, neg_n, created_at, updated_at
FROM taste_items WHERE id = ?;

-- name: GetTasteItemByRef :one
SELECT id, level, key, scope, window_start, source, polarity, pos_evidence, pos_n, neg_evidence, neg_n, created_at, updated_at
FROM taste_items WHERE level = ? AND key = ? AND scope = ? AND window_start = ?;

-- name: CreateTasteItem :one
INSERT INTO taste_items (level, key, scope, window_start, source, polarity, pos_evidence, pos_n, neg_evidence, neg_n, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: PutTasteItem :exec
INSERT INTO taste_items (id, level, key, scope, window_start, source, polarity, pos_evidence, pos_n, neg_evidence, neg_n, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    level = excluded.level, key = excluded.key, scope = excluded.scope, window_start = excluded.window_start,
    source = excluded.source, polarity = excluded.polarity,
    pos_evidence = excluded.pos_evidence, pos_n = excluded.pos_n,
    neg_evidence = excluded.neg_evidence, neg_n = excluded.neg_n,
    created_at = excluded.created_at, updated_at = excluded.updated_at;

-- name: DeleteTasteItem :exec
DELETE FROM taste_items WHERE id = ?;

-- name: ListStapleFoods :many
-- Recipes that are staples (THEME-1): pinned by the favourite star, or a main
-- dish (role main, cooked for lunch or dinner: sides, breakfasts and snacks
-- are no staples) with at least @min_sessions confirmed cook sessions since @sessions_since (a hidden
-- substitution variant's sessions count for its base recipe, a session counts
-- when a linked plate was not marked not_eaten), a positive signal, and no
-- dislike signal since @dislike_since.
SELECT f.id FROM foods f
WHERE f.kind = 'composed' AND f.archived = 0
  AND (f.favorite = 1 OR (
        f.role = 'main'
        AND (SELECT COUNT(*) FROM cook_sessions cs
          WHERE (cs.food_id = f.id OR cs.food_id IN (SELECT hv.food_id FROM hidden_variants hv WHERE hv.base_food_id = f.id))
            AND cs.state = 'confirmed' AND cs.cook_date >= sqlc.arg(sessions_since) AND cs.cook_date <= sqlc.arg(today)
            AND cs.cook_slot_id IN (SELECT id FROM time_slots WHERE name_key IN ('slot.lunch', 'slot.dinner'))
            AND EXISTS (SELECT 1 FROM plate_components pc
                        WHERE pc.cook_session_id = cs.id
                          AND NOT EXISTS (SELECT 1 FROM plate_feedback pf
                                          WHERE pf.plate_id = pc.plate_id AND pf.status = 'not_eaten'))
        ) >= sqlc.arg(min_sessions)
        AND EXISTS (SELECT 1 FROM taste_signals ts WHERE ts.food_id = f.id AND ts.kind IN ('loved', 'favourite', 'plate_added'))
        AND NOT EXISTS (SELECT 1 FROM taste_signals ts WHERE ts.food_id = f.id AND ts.kind IN ('disliked', 'not_for_me') AND ts.at >= sqlc.arg(dislike_since))))
ORDER BY f.id;
