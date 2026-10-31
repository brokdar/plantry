-- +goose Up
-- ai_conversations.week_id outlived the weeks table (00016) and the chat's
-- week scope (Phase 2 B2); nothing reads or writes it.
DROP INDEX ix_ai_conversations_week;
ALTER TABLE ai_conversations DROP COLUMN week_id;

-- +goose Down
ALTER TABLE ai_conversations ADD COLUMN week_id INTEGER;
CREATE INDEX ix_ai_conversations_week ON ai_conversations(week_id);
