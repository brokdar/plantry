-- name: GetLibraryReview :one
SELECT month, findings_json, card_dismissed FROM library_review WHERE id = 1;

-- name: SaveLibraryReview :exec
INSERT INTO library_review (id, month, findings_json, card_dismissed)
VALUES (1, sqlc.arg(month), sqlc.arg(findings_json), sqlc.arg(card_dismissed))
ON CONFLICT (id) DO UPDATE SET month = excluded.month, findings_json = excluded.findings_json,
    card_dismissed = excluded.card_dismissed;

-- name: ListReviewRecipes :many
SELECT f.id, f.name, f.role, f.prep_minutes, f.cook_minutes, f.variant_group_id,
       (SELECT COUNT(*) FROM food_instructions i WHERE i.food_id = f.id) AS steps
FROM foods f
WHERE f.kind = 'composed' AND f.archived = 0
  AND f.id NOT IN (SELECT food_id FROM hidden_variants)
ORDER BY f.id;

-- name: ListReviewIngredients :many
SELECT fc.parent_id, fc.child_id
FROM food_components fc
JOIN foods p ON p.id = fc.parent_id AND p.kind = 'composed' AND p.archived = 0
ORDER BY fc.parent_id, fc.child_id;

-- name: ListReplacedCounts :many
SELECT food_id, COUNT(DISTINCT CASE WHEN kind = 'plate_removed' THEN 'p' || plate_id ELSE 'r' || at END) AS replaced
FROM taste_signals
WHERE food_id IS NOT NULL AND at >= sqlc.arg(since)
  AND (kind = 'plate_removed' OR (kind = 'review_swap' AND weight < 0))
GROUP BY food_id;

-- name: SetFoodVariantGroup :exec
UPDATE foods SET variant_group_id = sqlc.arg(group_id) WHERE id = sqlc.arg(id) AND kind = 'composed';

-- name: AddLibraryGap :exec
INSERT INTO library_gaps (kind, dish, created_at) VALUES (sqlc.arg(kind), sqlc.arg(dish), sqlc.arg(created_at))
ON CONFLICT (kind) DO NOTHING;

-- name: ListLibraryGaps :many
SELECT kind, dish, find_id, offered FROM library_gaps ORDER BY created_at, kind;

-- name: DeleteLibraryGap :exec
DELETE FROM library_gaps WHERE kind = sqlc.arg(kind);

-- name: StartLibraryGap :exec
UPDATE library_gaps SET find_id = sqlc.arg(find_id), offered = 1 WHERE kind = sqlc.arg(kind);

-- name: FreeLibraryGap :exec
UPDATE library_gaps SET find_id = NULL WHERE kind = sqlc.arg(kind);

-- name: NewGapSession :exec
UPDATE library_gaps SET offered = 0;
