-- +goose Up
-- +goose StatementBegin
-- Replayable LLM calls (Phase 1, D23/D24). Each llm_call step stores the
-- request delta and parameters (request_json) and the reply (response_json);
-- tool definitions are stored once per distinct hash in ai_tool_schemas.
-- Runs carry the chat conversation and the HTTP request id.
ALTER TABLE ai_runs ADD COLUMN conversation_id INTEGER;
ALTER TABLE ai_runs ADD COLUMN request_id TEXT;
ALTER TABLE ai_run_steps ADD COLUMN request_json TEXT;
ALTER TABLE ai_run_steps ADD COLUMN response_json TEXT;
ALTER TABLE ai_run_steps ADD COLUMN stop_reason TEXT;
CREATE TABLE ai_tool_schemas (hash TEXT PRIMARY KEY, json TEXT NOT NULL);
CREATE INDEX ix_ai_runs_conversation ON ai_runs(conversation_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS ix_ai_runs_conversation;
DROP TABLE IF EXISTS ai_tool_schemas;
ALTER TABLE ai_run_steps DROP COLUMN stop_reason;
ALTER TABLE ai_run_steps DROP COLUMN response_json;
ALTER TABLE ai_run_steps DROP COLUMN request_json;
ALTER TABLE ai_runs DROP COLUMN request_id;
ALTER TABLE ai_runs DROP COLUMN conversation_id;
-- +goose StatementEnd
