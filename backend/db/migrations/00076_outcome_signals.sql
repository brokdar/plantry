-- +goose Up
-- +goose StatementBegin
-- Outcome signals the report reads (R6 outcomes). Neither table is the AI run
-- log: ai_runs is pruned by ai.run_retention_days, a verdict or an outcome
-- is kept with the recipe. run_id is not a foreign key for that reason.
--
-- chef_help_verdicts: the one-tap "Helped" / "Didn't help" under a Chef help
-- answer (run_id is the chef_help run). kind is the help request's kind
-- (explain_step, went_wrong), food_id the recipe being cooked.
CREATE TABLE chef_help_verdicts (
    run_id     INTEGER PRIMARY KEY,
    food_id    INTEGER REFERENCES foods(id) ON DELETE SET NULL,
    kind       TEXT NOT NULL DEFAULT '',
    verdict    TEXT NOT NULL CHECK (verdict IN ('helped', 'not_helped')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- import_check_outcomes: what the user did with one import check's findings
-- when the recipe was saved: the kinds of change kept and undone (JSON
-- arrays of strings), the finishing touch (none, added, skipped) and the
-- flags shown (flags are never applied or dismissed, so a saved recipe
-- ignored them).
CREATE TABLE import_check_outcomes (
    run_id     INTEGER PRIMARY KEY,
    food_id    INTEGER REFERENCES foods(id) ON DELETE SET NULL,
    kept       TEXT NOT NULL DEFAULT '[]',
    undone     TEXT NOT NULL DEFAULT '[]',
    touch      TEXT NOT NULL DEFAULT 'none' CHECK (touch IN ('none', 'added', 'skipped')),
    flags      INTEGER NOT NULL DEFAULT 0 CHECK (flags >= 0),
    created_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
DROP TABLE import_check_outcomes;
DROP TABLE chef_help_verdicts;
