-- +goose Up
-- Chef C2: recipe_suggestions evolves into the finds table (spec §7). A
-- find is a suggestion row with mode 'find' and a FIND-4 state; Discover
-- rows (daily/explore/known) keep status and get a state too once they are
-- finds. New columns: trigger (FIND-1, validated in Go), state, sources with
-- ratings, cross-check flags, origin label, changes, gate results and the
-- pipeline steps; the cost is the run's (ai_run_id).
-- Pending Discover suggestions become ready finds; their stored images are
-- not shown for finds. Nothing references this table, so a plain rebuild is
-- safe (the mode CHECK gains 'find'; status gains 'hidden' for a find
-- Discover does not show: searching or failed).
CREATE TABLE recipe_suggestions_new (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    mode             TEXT    NOT NULL CHECK (mode IN ('daily', 'explore', 'known', 'find')),
    query            TEXT,
    source_url       TEXT    NOT NULL,
    draft_json       TEXT    NOT NULL,
    annotations_json TEXT    NOT NULL,
    image_path       TEXT,
    grounded         INTEGER NOT NULL CHECK (grounded IN (0, 1)),
    status           TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected', 'hidden')),
    reason           TEXT    CHECK (reason IN ('too_long', 'ingredients', 'taste', 'looks_wrong')),
    ai_run_id        INTEGER REFERENCES ai_runs(id) ON DELETE SET NULL,
    food_id          INTEGER REFERENCES foods(id) ON DELETE SET NULL,
    shown_on         TEXT,
    created_at       TEXT    NOT NULL DEFAULT (datetime('now')),
    decided_at       TEXT,
    state            TEXT    CHECK (state IN ('searching', 'ready', 'needs_check', 'failed', 'in_library', 'discarded')),
    find_trigger     TEXT,
    sources_json     TEXT    NOT NULL DEFAULT '[]',
    flags_json       TEXT    NOT NULL DEFAULT '[]',
    origin           TEXT,
    changes_json     TEXT    NOT NULL DEFAULT '[]',
    gates_json       TEXT    NOT NULL DEFAULT '{}',
    steps_json       TEXT    NOT NULL DEFAULT '[]',
    questions_json   TEXT    NOT NULL DEFAULT '[]',
    updated_at       TEXT    NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO recipe_suggestions_new (
    id, mode, query, source_url, draft_json, annotations_json, image_path, grounded,
    status, reason, ai_run_id, food_id, shown_on, created_at, decided_at, state
)
SELECT id, mode, query, source_url, draft_json, annotations_json, image_path, grounded,
       status, reason, ai_run_id, food_id, shown_on, created_at, decided_at,
       CASE status WHEN 'pending' THEN 'ready' ELSE NULL END
FROM recipe_suggestions;
DROP TABLE recipe_suggestions;
ALTER TABLE recipe_suggestions_new RENAME TO recipe_suggestions;
CREATE INDEX idx_recipe_suggestions_source_url ON recipe_suggestions(source_url);
CREATE INDEX idx_recipe_suggestions_status ON recipe_suggestions(status, created_at);
CREATE UNIQUE INDEX idx_recipe_suggestions_shown_on ON recipe_suggestions(shown_on) WHERE shown_on IS NOT NULL;
CREATE INDEX idx_recipe_suggestions_state ON recipe_suggestions(state, updated_at) WHERE state IS NOT NULL;

-- source_rank: "looks wrong" lowers a site's rank (NEW-4); KIT-2/3 asked
-- state is one row per question key.
CREATE TABLE find_source_penalties (
    site       TEXT    PRIMARY KEY,
    count      INTEGER NOT NULL DEFAULT 0 CHECK (count >= 0),
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE kitchen_questions_asked (
    question   TEXT PRIMARY KEY,
    asked_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down
DROP TABLE kitchen_questions_asked;
DROP TABLE find_source_penalties;
DROP INDEX idx_recipe_suggestions_state;
DELETE FROM recipe_suggestions WHERE mode = 'find' OR status = 'hidden';
UPDATE recipe_suggestions SET reason = NULL WHERE reason = 'looks_wrong';
CREATE TABLE recipe_suggestions_old (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    mode             TEXT    NOT NULL CHECK (mode IN ('daily', 'explore', 'known')),
    query            TEXT,
    source_url       TEXT    NOT NULL,
    draft_json       TEXT    NOT NULL,
    annotations_json TEXT    NOT NULL,
    image_path       TEXT,
    grounded         INTEGER NOT NULL CHECK (grounded IN (0, 1)),
    status           TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    reason           TEXT    CHECK (reason IN ('too_long', 'ingredients', 'taste')),
    ai_run_id        INTEGER REFERENCES ai_runs(id) ON DELETE SET NULL,
    food_id          INTEGER REFERENCES foods(id) ON DELETE SET NULL,
    shown_on         TEXT,
    created_at       TEXT    NOT NULL DEFAULT (datetime('now')),
    decided_at       TEXT
);
INSERT INTO recipe_suggestions_old
SELECT id, mode, query, source_url, draft_json, annotations_json, image_path, grounded,
       status, reason, ai_run_id, food_id, shown_on, created_at, decided_at
FROM recipe_suggestions;
DROP TABLE recipe_suggestions;
ALTER TABLE recipe_suggestions_old RENAME TO recipe_suggestions;
CREATE INDEX idx_recipe_suggestions_source_url ON recipe_suggestions(source_url);
CREATE INDEX idx_recipe_suggestions_status ON recipe_suggestions(status, created_at);
CREATE UNIQUE INDEX idx_recipe_suggestions_shown_on ON recipe_suggestions(shown_on) WHERE shown_on IS NOT NULL;
