-- name: CreateAIRun :one
INSERT INTO ai_runs (kind, started_at, prompt_version, window_start, window_end, instruction)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: FinishAIRun :execresult
UPDATE ai_runs SET
    status             = ?,
    finished_at        = ?,
    provider           = ?,
    model              = ?,
    prompt_hash        = ?,
    iterations         = ?,
    input_tokens       = ?,
    output_tokens      = ?,
    cache_read_tokens  = ?,
    cache_write_tokens = ?,
    error_key          = ?
WHERE id = ?;

-- name: CreateAIRunStep :exec
INSERT INTO ai_run_steps (run_id, seq, type, name, args_json, result_json, is_error, duration_ms, input_tokens, output_tokens)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpsertAIPrompt :exec
INSERT INTO ai_prompts (hash, text) VALUES (?, ?)
ON CONFLICT(hash) DO NOTHING;
