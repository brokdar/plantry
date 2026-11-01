-- name: CreateAIRun :one
INSERT INTO ai_runs (kind, started_at, prompt_version, window_start, window_end, instruction, conversation_id, request_id,
    chooser, scope_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: FinishAIRun :execresult
-- prompt_hash resolves to NULL when the hash is not in ai_prompts, so a failed
-- UpsertPrompt never makes this UPDATE fail and leave the run 'running'.
UPDATE ai_runs SET
    status             = ?,
    finished_at        = ?,
    provider           = ?,
    model              = ?,
    prompt_hash        = (SELECT hash FROM ai_prompts WHERE hash = ?),
    iterations         = ?,
    input_tokens       = ?,
    output_tokens      = ?,
    cache_read_tokens  = ?,
    cache_write_tokens = ?,
    error_key          = ?,
    repair_attempts    = ?,
    violations_json    = ?,
    score_json         = ?,
    notes              = ?
WHERE id = ?;

-- name: CreateAIRunStep :exec
INSERT INTO ai_run_steps (run_id, seq, type, name, args_json, result_json, is_error, duration_ms, input_tokens, output_tokens,
    request_json, response_json, stop_reason)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpsertAIPrompt :exec
INSERT INTO ai_prompts (hash, text) VALUES (?, ?)
ON CONFLICT(hash) DO NOTHING;

-- name: UpsertAIToolSchema :exec
INSERT INTO ai_tool_schemas (hash, json) VALUES (?, ?)
ON CONFLICT(hash) DO NOTHING;

-- name: PruneAIRunSteps :execresult
-- Deletes up to 500 steps of runs started before the cutoff; the runs,
-- prompts and tool schemas stay. The repo repeats it until nothing is left,
-- so no single statement holds the write lock for long.
DELETE FROM ai_run_steps
WHERE id IN (
    SELECT s.id FROM ai_run_steps s
    JOIN ai_runs r ON r.id = s.run_id
    WHERE r.started_at < sqlc.arg(before)
    LIMIT 500
);

-- name: FailInterruptedAIRuns :execrows
-- At startup no run is running: a row still 'running' was cut off by a stop.
UPDATE ai_runs SET status = 'error', error_key = 'error.server', finished_at = sqlc.arg(finished_at)
WHERE status = 'running';

-- name: GetPlanRun :one
-- A generation run as GET /api/plan/runs/{id} shows it (B6).
SELECT id, kind, status, chooser, notes, error_key, scope_json, violations_json, score_json, window_start, window_end
FROM ai_runs WHERE id = ? AND kind IN ('generate', 'regenerate');

-- name: CountPlanRunRows :one
-- What a generation run wrote that still exists (B6).
SELECT
    (SELECT COUNT(*) FROM plates p WHERE p.ai_run_id = sqlc.arg(id)) AS plates,
    (SELECT COUNT(*) FROM cook_sessions s WHERE s.ai_run_id = sqlc.arg(id)) AS sessions,
    (SELECT COUNT(DISTINCT s.cook_date) FROM cook_sessions s WHERE s.ai_run_id = sqlc.arg(id)) AS cook_days;

-- name: GetRunNote :one
-- What the plan's draft summary shows of a run (D26).
SELECT chooser, notes FROM ai_runs WHERE id = ?;

-- name: SetRunOutcome :exec
-- D25: a run's outcome after its rows were accepted, discarded, un-accepted
-- (reset) or restored as drafts. open/NULL -> accepted or discarded; a run
-- that sees both is partially_accepted; reset undoes an accept (accepted ->
-- open, partially_accepted -> discarded); restored undoes a discard
-- (discarded -> open, partially_accepted -> accepted).
UPDATE ai_runs SET
    outcome = CASE
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'accepted' AND outcome IN ('discarded', 'partially_accepted') THEN 'partially_accepted'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'accepted' THEN 'accepted'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'discarded' AND outcome IN ('accepted', 'partially_accepted') THEN 'partially_accepted'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'discarded' THEN 'discarded'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'restored' AND outcome = 'discarded' THEN 'open'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'restored' AND outcome = 'partially_accepted' THEN 'accepted'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'restored' THEN outcome
        WHEN outcome = 'accepted' THEN 'open'
        WHEN outcome = 'partially_accepted' THEN 'discarded'
        ELSE outcome
    END,
    outcome_at = datetime('now')
WHERE id = sqlc.arg(id);

-- name: ListAIRuns :many
-- Runs newest first (by id), optionally filtered by kind, a started_at range
-- [from_ts, to_ts) and a cursor (ids below before_id). lim -1 lists all.
SELECT id, kind, started_at, finished_at, status, provider, model, prompt_version, chooser,
       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
       score_json, violations_json, outcome
FROM ai_runs
WHERE (CAST(sqlc.narg(kind) AS TEXT) IS NULL OR kind = sqlc.narg(kind))
  AND (CAST(sqlc.narg(from_ts) AS TEXT) IS NULL OR started_at >= sqlc.narg(from_ts))
  AND (CAST(sqlc.narg(to_ts) AS TEXT) IS NULL OR started_at < sqlc.narg(to_ts))
  AND (CAST(sqlc.narg(before_id) AS INTEGER) IS NULL OR id < sqlc.narg(before_id))
ORDER BY id DESC
LIMIT sqlc.arg(lim);

-- name: GetAIRunDetail :one
SELECT r.id, r.kind, r.started_at, r.finished_at, r.status, r.provider, r.model, r.prompt_version, r.chooser,
       r.input_tokens, r.output_tokens, r.cache_read_tokens, r.cache_write_tokens,
       r.score_json, r.violations_json, r.outcome,
       r.scope_json, r.instruction, r.notes, r.error_key, p.text AS prompt_text
FROM ai_runs r
LEFT JOIN ai_prompts p ON p.hash = r.prompt_hash
WHERE r.id = ?;

-- name: ListAIRunStepViews :many
-- A run's steps in seq order. args and result are cut to 8192 characters
-- (an llm_call shows its request_json and response_json there); the *_cut
-- flags say a value was longer.
SELECT seq, type, name, is_error, duration_ms, input_tokens, output_tokens,
       CAST(COALESCE(substr(COALESCE(args_json, request_json), 1, 8192), '') AS TEXT) AS args,
       COALESCE(length(COALESCE(args_json, request_json)), 0) > 8192 AS args_cut,
       CAST(COALESCE(substr(COALESCE(result_json, response_json), 1, 8192), '') AS TEXT) AS result,
       COALESCE(length(COALESCE(result_json, response_json)), 0) > 8192 AS result_cut
FROM ai_run_steps
WHERE run_id = ?
ORDER BY seq;

-- name: AIRunCorrections :many
-- D24/D12 per run with ids in [min_id, max_id]: the plates the run created
-- (a generator or agent create event), and how many of them got a correction
-- before the plate's first accept or confirm event: a user update or delete,
-- or an agent update or delete made by another run.
SELECT CAST(g.run_id AS INTEGER) AS run_id,
       COUNT(*) AS generated_plates,
       CAST(SUM(EXISTS (
           SELECT 1 FROM plate_events u
           WHERE u.plate_id = g.plate_id AND u.action IN ('update', 'delete')
             AND (u.actor = 'user' OR (u.actor = 'agent' AND u.ai_run_id IS NOT g.run_id))
             AND u.id > g.created_id
             AND u.id < COALESCE((SELECT MIN(a.id) FROM plate_events a
                                  WHERE a.plate_id = g.plate_id AND a.id > g.created_id
                                    AND a.action IN ('accept', 'confirm')), 9223372036854775807)
       )) AS INTEGER) AS corrected_plates
FROM (
    SELECT e.ai_run_id AS run_id, e.plate_id, MIN(e.id) AS created_id
    FROM plate_events e
    WHERE e.actor IN ('generator', 'agent') AND e.action = 'create' AND e.plate_id IS NOT NULL
      AND e.ai_run_id >= sqlc.arg(min_id) AND e.ai_run_id <= sqlc.arg(max_id)
    GROUP BY e.ai_run_id, e.plate_id
) g
GROUP BY g.run_id;

-- name: AIRunEdits :many
-- D12 per run with ids in [min_id, max_id]: the rows an agent run edited. A
-- session edit counts once as its session (its linked plates' events carry
-- cook_session_id and are skipped); every other plate counts as itself.
SELECT CAST(ai_run_id AS INTEGER) AS run_id,
       COUNT(DISTINCT CASE WHEN plate_id IS NULL THEN 's' || cook_session_id
                           WHEN cook_session_id IS NULL THEN 'p' || plate_id END) AS edits
FROM plate_events
WHERE actor = 'agent' AND ai_run_id >= sqlc.arg(min_id) AND ai_run_id <= sqlc.arg(max_id)
GROUP BY ai_run_id;

-- name: AgentRunsOfRows :many
-- D11: the agent runs that wrote to the given plates or sessions since the
-- row's last accept/confirm. A live accept/confirm (no later unlock) is not a
-- boundary yet, so Unaccept still reaches the runs it accepted; once undone,
-- the unlock is the boundary.
SELECT DISTINCT CAST(e.ai_run_id AS INTEGER) AS run_id FROM plate_events e
WHERE e.actor = 'agent' AND e.ai_run_id IS NOT NULL
  AND (e.plate_id IN (sqlc.slice(plate_ids)) OR e.cook_session_id IN (sqlc.slice(session_ids)))
  AND e.id > COALESCE((
    SELECT MAX(b.id) FROM plate_events b
    WHERE b.action IN ('accept', 'confirm', 'unlock')
      AND ((b.plate_id IS NOT NULL AND b.plate_id = e.plate_id) OR (b.cook_session_id IS NOT NULL AND b.cook_session_id = e.cook_session_id))
      AND NOT (b.action IN ('accept', 'confirm') AND NOT EXISTS (
        SELECT 1 FROM plate_events c
        WHERE c.id > b.id AND c.action IN ('accept', 'confirm', 'unlock')
          AND ((c.plate_id IS NOT NULL AND c.plate_id = e.plate_id) OR (c.cook_session_id IS NOT NULL AND c.cook_session_id = e.cook_session_id))))
  ), 0)
ORDER BY run_id;

-- name: ChatRunsOfRun :many
-- The chat runs whose plan-writing calls (ai_changes) wrote under run_id: a
-- chat run's regenerate call writes under the generator's own run, so the
-- chat run follows that run's outcome. run_ids_json is a JSON array of ids.
SELECT DISTINCT CAST(run_id AS INTEGER) AS run_id FROM ai_changes
WHERE instr(',' || replace(replace(replace(run_ids_json, '[', ''), ']', ''), ' ', '') || ',', ',' || CAST(sqlc.arg(run_id) AS TEXT) || ',') > 0
ORDER BY run_id;
