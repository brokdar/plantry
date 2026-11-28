-- Read-only aggregates for `plantry report`. from_ts is the window start in
-- the stored time layout ("2006-01-02 15:04:05", UTC). No free text is
-- selected: no prompts, no help questions, no recipe text.

-- name: ReportRunStats :many
-- Runs of the window by kind, chooser and status, with their cost.
SELECT kind, COALESCE(chooser, '') AS chooser, status, COUNT(*) AS n,
       CAST(COALESCE(SUM(cost_usd), 0) AS REAL) AS usd
FROM ai_runs
WHERE started_at >= sqlc.arg(from_ts)
GROUP BY kind, COALESCE(chooser, ''), status;

-- name: ReportRunErrors :many
SELECT kind, COALESCE(error_key, '') AS error_key, COUNT(*) AS n
FROM ai_runs
WHERE started_at >= sqlc.arg(from_ts) AND status = 'error'
GROUP BY kind, COALESCE(error_key, '')
ORDER BY kind, n DESC, error_key;

-- name: ReportRunDurations :many
SELECT kind, duration_ms
FROM ai_runs
WHERE started_at >= sqlc.arg(from_ts) AND duration_ms IS NOT NULL
ORDER BY kind, duration_ms;

-- name: ReportRunModels :many
SELECT provider, model, COUNT(*) AS n
FROM ai_runs
WHERE started_at >= sqlc.arg(from_ts) AND provider <> ''
GROUP BY provider, model
ORDER BY n DESC, provider, model;

-- name: ReportChefSpendByMonth :many
-- Capped Chef spend per calendar month from from_month ("2006-01") on.
SELECT substr(started_at, 1, 7) AS month, CAST(COALESCE(SUM(cost_usd), 0) AS REAL) AS usd
FROM ai_runs
WHERE substr(started_at, 1, 7) >= sqlc.arg(from_month) AND kind IN (sqlc.slice(kinds))
GROUP BY substr(started_at, 1, 7)
ORDER BY month;

-- name: ReportHelpVerdicts :many
-- Answered help runs of the window by verdict ('none' when not given).
SELECT COALESCE(v.verdict, 'none') AS verdict, COUNT(*) AS n
FROM ai_runs r
LEFT JOIN chef_help_verdicts v ON v.run_id = r.id
WHERE r.kind = 'chef_help' AND r.status = 'ok' AND r.started_at >= sqlc.arg(from_ts)
GROUP BY COALESCE(v.verdict, 'none');

-- name: ReportFindStates :many
SELECT COALESCE(state, '') AS state, status, COUNT(*) AS n
FROM recipe_suggestions
WHERE mode = 'find' AND created_at >= sqlc.arg(from_ts)
GROUP BY COALESCE(state, ''), status;

-- name: ReportFindRejectReasons :many
SELECT COALESCE(reason, '') AS reason, COUNT(*) AS n
FROM recipe_suggestions
WHERE mode = 'find' AND status = 'rejected' AND created_at >= sqlc.arg(from_ts)
GROUP BY COALESCE(reason, '')
ORDER BY n DESC, reason;

-- name: ReportFindFunnel :one
-- Accepted finds of the window: still a recipe (food_id is cleared when the
-- recipe is deleted), cooked at least once, eaten on a rated plate.
SELECT COUNT(*) AS accepted,
       CAST(COALESCE(SUM(CASE WHEN food_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS in_library,
       CAST(COALESCE(SUM(CASE WHEN EXISTS (
           SELECT 1 FROM cook_sessions cs WHERE cs.food_id = rs.food_id AND cs.cooked_at IS NOT NULL
       ) THEN 1 ELSE 0 END), 0) AS INTEGER) AS cooked,
       CAST(COALESCE(SUM(CASE WHEN EXISTS (
           SELECT 1 FROM plate_components pc JOIN plate_feedback pf ON pf.plate_id = pc.plate_id
           WHERE pc.food_id = rs.food_id
       ) THEN 1 ELSE 0 END), 0) AS INTEGER) AS rated
FROM recipe_suggestions rs
WHERE rs.mode = 'find' AND rs.status = 'accepted' AND rs.created_at >= sqlc.arg(from_ts);

-- name: ReportTrialFoods :one
-- Recipes the Chef put on trial (a snapshot, not windowed): still in the
-- library (not archived), archived, and cooked at least once.
SELECT COUNT(*) AS total,
       CAST(COALESCE(SUM(archived), 0) AS INTEGER) AS archived,
       CAST(COALESCE(SUM(CASE WHEN EXISTS (
           SELECT 1 FROM cook_sessions cs WHERE cs.food_id = f.id AND cs.cooked_at IS NOT NULL
       ) THEN 1 ELSE 0 END), 0) AS INTEGER) AS cooked
FROM foods f
WHERE trial = 1;

-- name: ReportImportOutcomes :many
SELECT kept, undone, touch, flags
FROM import_check_outcomes
WHERE created_at >= sqlc.arg(from_ts);

-- name: ReportJobs :many
SELECT kind, status, COUNT(*) AS n
FROM chef_jobs
WHERE updated_at >= sqlc.arg(from_ts)
GROUP BY kind, status
ORDER BY kind, status;

-- name: ReportClosingEvaluations :many
-- The closing plan.evaluation (last 'after' or 'final_proposal' step) of each
-- generate or regenerate run of the window.
SELECT r.id AS run_id, s.result_json
FROM ai_runs r
JOIN ai_run_steps s ON s.run_id = r.id AND s.name = 'plan.evaluation' AND s.is_error = 0
 AND s.seq = (
     SELECT MAX(s2.seq) FROM ai_run_steps s2
     WHERE s2.run_id = r.id AND s2.name = 'plan.evaluation' AND s2.is_error = 0
       AND json_extract(s2.args_json, '$.phase') IN ('after', 'final_proposal')
 )
WHERE r.kind IN ('generate', 'regenerate') AND r.started_at >= sqlc.arg(from_ts);

-- name: ReportRatings :many
-- Rated plates of the window by status, split by whether the plate holds a
-- dish the Chef brought (an accepted find or a trial recipe).
SELECT CAST(EXISTS (
           SELECT 1 FROM plate_components pc
           WHERE pc.plate_id = pf.plate_id
             AND (pc.food_id IN (SELECT food_id FROM recipe_suggestions WHERE mode = 'find' AND food_id IS NOT NULL)
                  OR pc.food_id IN (SELECT id FROM foods WHERE trial = 1))
       ) AS INTEGER) AS chef_origin,
       pf.status AS status, COUNT(*) AS n
FROM plate_feedback pf
WHERE pf.rated_at >= sqlc.arg(from_ts)
GROUP BY 1, pf.status;
