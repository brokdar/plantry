-- +goose Up
-- AI connection fix: run duration at ms precision and per-step metrics.
-- duration_ms is NULL for runs made before this migration (and while running).
-- Step started_at has ms precision; ttft_ms is the time to the first streamed
-- token or tool call (NULL for tool calls and calls without one);
-- reasoning_tokens is what the provider reports (OpenAI; Anthropic counts
-- thinking inside output_tokens, so thinking_chars records the thinking text
-- length instead); served_model is the model the provider answered with.
ALTER TABLE ai_runs ADD COLUMN duration_ms INTEGER;
ALTER TABLE ai_runs ADD COLUMN reasoning_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_run_steps ADD COLUMN started_at TEXT;
ALTER TABLE ai_run_steps ADD COLUMN ttft_ms INTEGER;
ALTER TABLE ai_run_steps ADD COLUMN cache_read_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_run_steps ADD COLUMN cache_write_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_run_steps ADD COLUMN reasoning_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_run_steps ADD COLUMN thinking_chars INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_run_steps ADD COLUMN served_model TEXT;

-- +goose Down
ALTER TABLE ai_run_steps DROP COLUMN served_model;
ALTER TABLE ai_run_steps DROP COLUMN thinking_chars;
ALTER TABLE ai_run_steps DROP COLUMN reasoning_tokens;
ALTER TABLE ai_run_steps DROP COLUMN cache_write_tokens;
ALTER TABLE ai_run_steps DROP COLUMN cache_read_tokens;
ALTER TABLE ai_run_steps DROP COLUMN ttft_ms;
ALTER TABLE ai_run_steps DROP COLUMN started_at;
ALTER TABLE ai_runs DROP COLUMN reasoning_tokens;
ALTER TABLE ai_runs DROP COLUMN duration_ms;
