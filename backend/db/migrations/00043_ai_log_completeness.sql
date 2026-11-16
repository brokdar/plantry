-- +goose Up
-- AI run log completeness: the provider's request id of every LLM call (also
-- on success) and the app build that made a run.
ALTER TABLE ai_run_steps ADD COLUMN provider_request_id TEXT;
ALTER TABLE ai_runs ADD COLUMN app_version TEXT;

-- +goose Down
ALTER TABLE ai_runs DROP COLUMN app_version;
ALTER TABLE ai_run_steps DROP COLUMN provider_request_id;
