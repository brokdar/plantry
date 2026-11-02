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
