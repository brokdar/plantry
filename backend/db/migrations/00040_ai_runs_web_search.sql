-- +goose Up
-- Phase 5 B2: server-side web searches the provider billed, per run (summed)
-- and per llm_call step. Priced on read (airun.Price.PerSearch).
ALTER TABLE ai_runs ADD COLUMN web_search_requests INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_run_steps ADD COLUMN web_search_requests INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE ai_run_steps DROP COLUMN web_search_requests;
ALTER TABLE ai_runs DROP COLUMN web_search_requests;
