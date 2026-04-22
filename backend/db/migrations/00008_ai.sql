-- +goose Up
CREATE TABLE ai_conversations (
    id          INTEGER PRIMARY KEY,
    week_id     INTEGER REFERENCES weeks(id) ON DELETE SET NULL,
    title       TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX ix_ai_conversations_week ON ai_conversations(week_id);

CREATE TABLE ai_messages (
    id              INTEGER PRIMARY KEY,
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('system','user','assistant','tool','error')),
    content         TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX ix_ai_messages_conversation ON ai_messages(conversation_id);

-- +goose Down
DROP INDEX IF EXISTS ix_ai_messages_conversation;
DROP TABLE IF EXISTS ai_messages;
DROP INDEX IF EXISTS ix_ai_conversations_week;
DROP TABLE IF EXISTS ai_conversations;
