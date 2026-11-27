-- name: CreateFind :one
-- A requested or autonomous find starts searching (FIND-4).
INSERT INTO recipe_suggestions (mode, query, source_url, draft_json, annotations_json, grounded, status, state, find_trigger)
VALUES ('find', sqlc.arg(query), '', '{}', '{}', 0, 'hidden', 'searching', sqlc.arg(find_trigger))
RETURNING *;

-- name: FinishFind :one
-- The pipeline's outcome: state and everything it learned. A ready or
-- needs_check find shows as pending in Discover; a failed one stays hidden.
UPDATE recipe_suggestions SET
    state = sqlc.arg(state), source_url = sqlc.arg(source_url), draft_json = sqlc.arg(draft_json),
    sources_json = sqlc.arg(sources_json), flags_json = sqlc.arg(flags_json), origin = sqlc.narg(origin),
    changes_json = sqlc.arg(changes_json), gates_json = sqlc.arg(gates_json), steps_json = sqlc.arg(steps_json),
    questions_json = sqlc.arg(questions_json),
    ai_run_id = sqlc.narg(ai_run_id),
    status = CASE WHEN sqlc.arg(state) = 'failed' THEN 'hidden' ELSE 'pending' END,
    updated_at = datetime('now')
WHERE id = sqlc.arg(id) AND state = 'searching'
RETURNING *;

-- name: ListFinds :many
-- Finds (rows with a state), optionally of one state, newest first.
SELECT * FROM recipe_suggestions
WHERE state IS NOT NULL
  AND (CAST(sqlc.narg(state) AS TEXT) IS NULL OR state = CAST(sqlc.narg(state) AS TEXT))
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(lim);

-- name: DecideFind :one
-- Keep (in_library, with the saved food) or discard a ready or needs_check
-- find; no row when it is decided already.
UPDATE recipe_suggestions SET
    state = sqlc.arg(state), reason = sqlc.narg(reason), food_id = sqlc.narg(food_id),
    status = CASE WHEN sqlc.arg(state) = 'in_library' THEN 'accepted' ELSE 'rejected' END,
    decided_at = datetime('now'), updated_at = datetime('now')
WHERE id = sqlc.arg(id) AND state IN ('ready', 'needs_check')
RETURNING *;

-- name: PenaliseSource :exec
-- "Looks wrong" lowers the site's rank (NEW-4).
INSERT INTO find_source_penalties (site, count) VALUES (?, 1)
ON CONFLICT (site) DO UPDATE SET count = count + 1, updated_at = datetime('now');

-- name: ListSourcePenalties :many
SELECT site, count FROM find_source_penalties;

-- name: MarkKitchenQuestionAsked :exec
-- KIT-2/3: a question is asked once.
INSERT INTO kitchen_questions_asked (question) VALUES (?) ON CONFLICT (question) DO NOTHING;

-- name: ListKitchenQuestionsAsked :many
SELECT question FROM kitchen_questions_asked ORDER BY question;

-- name: ListStrandedFindIDs :many
-- Searching finds no chef_find job is queued or running for (the job failed
-- at boot recovery or was lost).
SELECT s.id FROM recipe_suggestions s
WHERE s.state = 'searching'
  AND NOT EXISTS (
    SELECT 1 FROM chef_jobs j
    WHERE j.kind = 'chef_find' AND j.payload = CAST(s.id AS TEXT) AND j.status IN ('queued', 'running')
  );
