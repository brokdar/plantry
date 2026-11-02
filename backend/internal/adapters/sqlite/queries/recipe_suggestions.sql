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

-- name: DecideRecipeSuggestion :one
-- Accept or reject a pending suggestion; no row when it is decided already.
UPDATE recipe_suggestions SET
    status = sqlc.arg(status), reason = sqlc.narg(reason), food_id = sqlc.narg(food_id),
    decided_at = datetime('now')
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: SetDiscoveryRunOutcome :exec
-- D11: any suggestion accepted -> accepted; all rejected -> discarded; else
-- open. A run without suggestions keeps NULL.
UPDATE ai_runs SET
    outcome = (
        SELECT CASE
            WHEN COUNT(*) = 0 THEN NULL
            WHEN SUM(s.status = 'accepted') > 0 THEN 'accepted'
            WHEN SUM(s.status = 'rejected') = COUNT(*) THEN 'discarded'
            ELSE 'open'
        END
        FROM recipe_suggestions s WHERE s.ai_run_id = CAST(sqlc.arg(id) AS INTEGER)
    ),
    outcome_at = datetime('now')
WHERE id = sqlc.arg(id) AND kind = 'discovery';

-- name: ListRecipeSuggestions :many
-- Suggestions, optionally of one status and run, last decided (else newest)
-- first.
SELECT * FROM recipe_suggestions
WHERE (CAST(sqlc.narg(status) AS TEXT) IS NULL OR status = CAST(sqlc.narg(status) AS TEXT))
  AND (CAST(sqlc.narg(run_id) AS INTEGER) IS NULL OR ai_run_id = CAST(sqlc.narg(run_id) AS INTEGER))
ORDER BY COALESCE(decided_at, created_at) DESC, id DESC
LIMIT sqlc.arg(lim);

-- name: DeletePendingSuggestionsBefore :many
-- D12: pending suggestions created before the cutoff go; their ids let the
-- caller delete the images.
DELETE FROM recipe_suggestions
WHERE status = 'pending' AND created_at < sqlc.arg(before)
RETURNING id;
