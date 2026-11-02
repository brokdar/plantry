-- name: CreateRecipeSuggestion :one
INSERT INTO recipe_suggestions (
    mode, query, source_url, draft_json, annotations_json, grounded, ai_run_id, shown_on
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRecipeSuggestion :one
SELECT * FROM recipe_suggestions WHERE id = ?;

-- name: SetRecipeSuggestionImage :exec
UPDATE recipe_suggestions SET image_path = ? WHERE id = ?;

-- name: SourceURLKnown :one
-- Whether a food or an earlier suggestion (any status) already has this URL.
SELECT CAST((
    EXISTS (SELECT 1 FROM foods f WHERE f.source_url = CAST(sqlc.arg(url) AS TEXT))
    OR EXISTS (SELECT 1 FROM recipe_suggestions s WHERE s.source_url = CAST(sqlc.arg(url) AS TEXT))
) AS INTEGER) AS known;

-- name: DiscoverySuggestionCounts :many
-- B7: per discovery run with an id in [min_id, max_id], the suggestions it
-- stored and how many of them the user accepted.
SELECT CAST(ai_run_id AS INTEGER) AS run_id,
       COUNT(*) AS suggestions,
       CAST(COALESCE(SUM(status = 'accepted'), 0) AS INTEGER) AS accepted
FROM recipe_suggestions
WHERE ai_run_id >= sqlc.arg(min_id) AND ai_run_id <= sqlc.arg(max_id)
GROUP BY ai_run_id;
