-- +goose Up
-- +goose StatementBegin
-- Programmatic tool calling: a `code_execution` step type (the code the model
-- ran, its stdout/stderr/return code), who called each tool_call step
-- (caller: direct | code_execution, caller_tool_id: the code's server tool
-- use id), the run's code execution container, and the container a chat
-- conversation keeps between turns. SQLite cannot alter a CHECK, so the steps
-- table is rebuilt (nothing references it).
CREATE TABLE ai_run_steps_new (
    id                  INTEGER PRIMARY KEY,
    run_id              INTEGER NOT NULL REFERENCES ai_runs(id) ON DELETE CASCADE,
    seq                 INTEGER NOT NULL,
    type                TEXT NOT NULL CHECK (type IN ('llm_call','tool_call','code_execution')),
    name                TEXT NOT NULL,
    args_json           TEXT,
    result_json         TEXT,
    is_error            INTEGER NOT NULL DEFAULT 0,
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    input_tokens        INTEGER NOT NULL DEFAULT 0,
    output_tokens       INTEGER NOT NULL DEFAULT 0,
    request_json        TEXT,
    response_json       TEXT,
    stop_reason         TEXT,
    web_search_requests INTEGER NOT NULL DEFAULT 0,
    started_at          TEXT,
    ttft_ms             INTEGER,
    cache_read_tokens   INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens  INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens    INTEGER NOT NULL DEFAULT 0,
    thinking_chars      INTEGER NOT NULL DEFAULT 0,
    served_model        TEXT,
    provider_request_id TEXT,
    caller              TEXT,
    caller_tool_id      TEXT,
    UNIQUE (run_id, seq)
);
INSERT INTO ai_run_steps_new (id, run_id, seq, type, name, args_json, result_json, is_error, duration_ms, input_tokens,
    output_tokens, request_json, response_json, stop_reason, web_search_requests, started_at, ttft_ms, cache_read_tokens,
    cache_write_tokens, reasoning_tokens, thinking_chars, served_model, provider_request_id)
SELECT id, run_id, seq, type, name, args_json, result_json, is_error, duration_ms, input_tokens,
    output_tokens, request_json, response_json, stop_reason, web_search_requests, started_at, ttft_ms, cache_read_tokens,
    cache_write_tokens, reasoning_tokens, thinking_chars, served_model, provider_request_id
FROM ai_run_steps;
DROP TABLE ai_run_steps;
ALTER TABLE ai_run_steps_new RENAME TO ai_run_steps;
ALTER TABLE ai_runs ADD COLUMN container_id TEXT;
ALTER TABLE ai_conversations ADD COLUMN container_id TEXT;
ALTER TABLE ai_conversations ADD COLUMN container_expires_at TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE ai_conversations DROP COLUMN container_expires_at;
ALTER TABLE ai_conversations DROP COLUMN container_id;
ALTER TABLE ai_runs DROP COLUMN container_id;
DELETE FROM ai_run_steps WHERE type = 'code_execution';
CREATE TABLE ai_run_steps_old (
    id                  INTEGER PRIMARY KEY,
    run_id              INTEGER NOT NULL REFERENCES ai_runs(id) ON DELETE CASCADE,
    seq                 INTEGER NOT NULL,
    type                TEXT NOT NULL CHECK (type IN ('llm_call','tool_call')),
    name                TEXT NOT NULL,
    args_json           TEXT,
    result_json         TEXT,
    is_error            INTEGER NOT NULL DEFAULT 0,
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    input_tokens        INTEGER NOT NULL DEFAULT 0,
    output_tokens       INTEGER NOT NULL DEFAULT 0,
    request_json        TEXT,
    response_json       TEXT,
    stop_reason         TEXT,
    web_search_requests INTEGER NOT NULL DEFAULT 0,
    started_at          TEXT,
    ttft_ms             INTEGER,
    cache_read_tokens   INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens  INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens    INTEGER NOT NULL DEFAULT 0,
    thinking_chars      INTEGER NOT NULL DEFAULT 0,
    served_model        TEXT,
    provider_request_id TEXT,
    UNIQUE (run_id, seq)
);
INSERT INTO ai_run_steps_old (id, run_id, seq, type, name, args_json, result_json, is_error, duration_ms, input_tokens,
    output_tokens, request_json, response_json, stop_reason, web_search_requests, started_at, ttft_ms, cache_read_tokens,
    cache_write_tokens, reasoning_tokens, thinking_chars, served_model, provider_request_id)
SELECT id, run_id, seq, type, name, args_json, result_json, is_error, duration_ms, input_tokens,
    output_tokens, request_json, response_json, stop_reason, web_search_requests, started_at, ttft_ms, cache_read_tokens,
    cache_write_tokens, reasoning_tokens, thinking_chars, served_model, provider_request_id
FROM ai_run_steps;
DROP TABLE ai_run_steps;
ALTER TABLE ai_run_steps_old RENAME TO ai_run_steps;
-- +goose StatementEnd
