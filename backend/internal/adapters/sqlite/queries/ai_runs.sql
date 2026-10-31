-- name: CreateAIRun :one
INSERT INTO ai_runs (kind, started_at, prompt_version, window_start, window_end, instruction, conversation_id, request_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
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
    error_key          = ?
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

-- name: SetRunOutcome :exec
-- D25: a run's outcome after its rows were accepted, discarded or un-accepted
-- (reset). open/NULL -> accepted or discarded; a run that sees both is
-- partially_accepted; reset undoes an accept (accepted -> open,
-- partially_accepted -> discarded).
UPDATE ai_runs SET
    outcome = CASE
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'accepted' AND outcome IN ('discarded', 'partially_accepted') THEN 'partially_accepted'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'accepted' THEN 'accepted'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'discarded' AND outcome IN ('accepted', 'partially_accepted') THEN 'partially_accepted'
        WHEN CAST(sqlc.arg(event) AS TEXT) = 'discarded' THEN 'discarded'
        WHEN outcome = 'accepted' THEN 'open'
        WHEN outcome = 'partially_accepted' THEN 'discarded'
        ELSE outcome
    END,
    outcome_at = datetime('now')
WHERE id = sqlc.arg(id);
