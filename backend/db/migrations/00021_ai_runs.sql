-- +goose Up
-- +goose StatementBegin
-- Run log for AI features: one ai_runs row per user-visible AI action, one
-- ai_run_steps row per LLM or tool call. System prompts are stored once per
-- distinct text in ai_prompts.
CREATE TABLE ai_prompts (
    hash TEXT PRIMARY KEY,
    text TEXT NOT NULL
);

CREATE TABLE ai_runs (
    id                 INTEGER PRIMARY KEY,
    kind               TEXT NOT NULL CHECK (kind IN ('generate','regenerate','chat','discovery','classify','import_fallback','lookup','untracked')),
    started_at         TEXT NOT NULL,
    finished_at        TEXT,
    status             TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','ok','error','iterations_exceeded','cancelled','partial')),
    provider           TEXT NOT NULL DEFAULT '',
    model              TEXT NOT NULL DEFAULT '',
    prompt_version     TEXT NOT NULL DEFAULT '',
    prompt_hash        TEXT REFERENCES ai_prompts(hash),
    window_start       TEXT,
    window_end         TEXT,
    scope_json         TEXT,
    instruction        TEXT,
    chooser            TEXT CHECK (chooser IS NULL OR chooser IN ('llm','heuristic')),
    iterations         INTEGER NOT NULL DEFAULT 0,
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    repair_attempts    INTEGER NOT NULL DEFAULT 0,
    violations_json    TEXT,
    score_json         TEXT,
    judge_json         TEXT,
    outcome            TEXT CHECK (outcome IS NULL OR outcome IN ('accepted','discarded','partially_accepted','open')),
    outcome_at         TEXT,
    error_key          TEXT
);

CREATE INDEX ix_ai_runs_started ON ai_runs(started_at);

CREATE TABLE ai_run_steps (
    id            INTEGER PRIMARY KEY,
    run_id        INTEGER NOT NULL REFERENCES ai_runs(id) ON DELETE CASCADE,
    seq           INTEGER NOT NULL,
    type          TEXT NOT NULL CHECK (type IN ('llm_call','tool_call')),
    name          TEXT NOT NULL,
    args_json     TEXT,
    result_json   TEXT,
    is_error      INTEGER NOT NULL DEFAULT 0,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    UNIQUE (run_id, seq)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS ai_run_steps;
DROP INDEX IF EXISTS ix_ai_runs_started;
DROP TABLE IF EXISTS ai_runs;
DROP TABLE IF EXISTS ai_prompts;
-- +goose StatementEnd
