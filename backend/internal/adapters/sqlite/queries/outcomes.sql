-- name: UpsertHelpVerdict :execrows
-- Keeps the verdict of a chef_help run with its recipe (from the run's
-- scope: the cook session) and request kind. No row is written for a run that
-- is not a chef_help run.
INSERT INTO chef_help_verdicts (run_id, food_id, kind, verdict, created_at, updated_at)
SELECT r.id, cs.food_id, COALESCE(json_extract(r.scope_json, '$.kind'), ''), sqlc.arg(verdict), sqlc.arg(now), sqlc.arg(now)
FROM ai_runs r
LEFT JOIN cook_sessions cs ON cs.id = json_extract(r.scope_json, '$.session_id')
WHERE r.id = sqlc.arg(run_id) AND r.kind = 'chef_help'
ON CONFLICT(run_id) DO UPDATE SET verdict = excluded.verdict, updated_at = excluded.updated_at;

-- name: UpsertImportCheckOutcome :exec
INSERT INTO import_check_outcomes (run_id, food_id, kept, undone, touch, flags, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO UPDATE SET
    food_id = excluded.food_id, kept = excluded.kept, undone = excluded.undone,
    touch = excluded.touch, flags = excluded.flags;
