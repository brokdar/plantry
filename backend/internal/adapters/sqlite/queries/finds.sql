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

-- name: PlaceFind :one
-- The planner placed a ready find (FIND-4 ready -> in_library) in the
-- window starting on placed_on; no row when it is not ready.
UPDATE recipe_suggestions SET
    state = 'in_library', status = 'accepted', food_id = sqlc.arg(food_id), placed_on = sqlc.arg(placed_on),
    decided_at = datetime('now'), updated_at = datetime('now')
WHERE id = sqlc.arg(id) AND state = 'ready'
RETURNING *;

-- name: UnplaceFind :one
-- A placed find goes back to ready (the run did not use it, or Undo).
UPDATE recipe_suggestions SET
    state = 'ready', status = 'pending', food_id = NULL, placed_on = NULL,
    decided_at = NULL, updated_at = datetime('now')
WHERE id = sqlc.arg(id) AND state = 'in_library' AND placed_on IS NOT NULL
RETURNING *;

-- name: ListPlacedFinds :many
-- The finds the planner placed in the window starting on placed_on.
SELECT * FROM recipe_suggestions
WHERE placed_on = sqlc.arg(placed_on) AND state = 'in_library'
ORDER BY id;

-- name: ShowNextReadyFind :one
-- FIND-2: with the Chef on the daily idea is a ready find, never shown
-- before, after the given id.
UPDATE recipe_suggestions SET shown_on = sqlc.arg(day)
WHERE id = (
    SELECT s.id FROM recipe_suggestions s
    WHERE s.state = 'ready' AND s.shown_on IS NULL AND s.id > sqlc.arg(after)
    ORDER BY s.id LIMIT 1
)
RETURNING *;

-- name: TouchPlanningSession :exec
INSERT INTO chef_planning_session (id, until) VALUES (1, sqlc.arg(until))
ON CONFLICT (id) DO UPDATE SET until = MAX(until, excluded.until);

-- name: GetPlanningSession :one
SELECT until FROM chef_planning_session WHERE id = 1;

-- name: BlockSearchDomain :exec
INSERT INTO find_search_blocks (domain, blocked_until) VALUES (sqlc.arg(domain), sqlc.arg(blocked_until))
ON CONFLICT (domain) DO UPDATE SET blocked_until = excluded.blocked_until;

-- name: ListSearchBlocked :many
SELECT domain FROM find_search_blocks WHERE blocked_until > sqlc.arg(now);
