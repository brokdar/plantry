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
