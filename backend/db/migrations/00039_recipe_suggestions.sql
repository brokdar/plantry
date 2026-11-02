-- +goose Up
-- Phase 5 D13: recipe suggestions found by discovery. A row is the verified
-- draft (draft_json, the importer's Draft) with its local annotations
-- (annotations_json), the stored image (suggestions/<id>.jpg) and the
-- outcome. Accepted and rejected rows are kept: they dedupe later runs and
-- feed the research prompt. A run or food that goes away leaves the row.
CREATE TABLE recipe_suggestions (
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
CREATE INDEX idx_recipe_suggestions_source_url ON recipe_suggestions(source_url);
CREATE INDEX idx_recipe_suggestions_status ON recipe_suggestions(status, created_at);
CREATE UNIQUE INDEX idx_recipe_suggestions_shown_on ON recipe_suggestions(shown_on) WHERE shown_on IS NOT NULL;

-- +goose Down
DROP TABLE recipe_suggestions;
