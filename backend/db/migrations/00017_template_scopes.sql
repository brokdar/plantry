-- +goose Up
-- Add scope to templates and rebuild template_components into template_entries
-- with new slot_id + note columns. All legacy templates land at scope='slot':
-- the old schema had no per-entry slot binding, so legacy entries arrive with
-- slot_id=NULL and would be unusable under day/week scope semantics. Slot
-- scope's apply path groups by day_offset so legacy multi-offset templates
-- continue to work; users re-save once to upgrade to true week scope.
ALTER TABLE templates ADD COLUMN scope TEXT NOT NULL DEFAULT 'slot'
    CHECK (scope IN ('slot','day','week'));

CREATE TABLE template_entries (
    id           INTEGER PRIMARY KEY,
    template_id  INTEGER NOT NULL REFERENCES templates(id)   ON DELETE CASCADE,
    food_id      INTEGER NOT NULL REFERENCES foods(id)       ON DELETE RESTRICT,
    portions     REAL    NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0,
    day_offset   INTEGER NOT NULL DEFAULT 0,
    slot_id      INTEGER REFERENCES time_slots(id)           ON DELETE RESTRICT,
    note         TEXT
);
INSERT INTO template_entries (id, template_id, food_id, portions, sort_order, day_offset, slot_id, note)
    SELECT id, template_id, food_id, portions, sort_order, day_offset, NULL, NULL FROM template_components;
DROP INDEX IF EXISTS ix_template_components_template;
DROP TABLE template_components;
CREATE INDEX ix_template_entries_template ON template_entries(template_id);

-- +goose Down
-- Revert: rebuild template_components from template_entries (drops slot_id + note
-- silently — any data in those columns is lost), then drop scope from templates.
-- Single-user LAN deployment: rollback is best-effort.
CREATE TABLE template_components (
    id           INTEGER PRIMARY KEY,
    template_id  INTEGER NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    food_id      INTEGER NOT NULL REFERENCES foods(id)    ON DELETE RESTRICT,
    portions     REAL    NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0,
    day_offset   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO template_components (id, template_id, food_id, portions, sort_order, day_offset)
    SELECT id, template_id, food_id, portions, sort_order, day_offset FROM template_entries;
DROP INDEX IF EXISTS ix_template_entries_template;
DROP TABLE template_entries;
CREATE INDEX ix_template_components_template ON template_components(template_id);

CREATE TABLE templates_old (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO templates_old (id, name, created_at)
    SELECT id, name, created_at FROM templates;
DROP TABLE templates;
ALTER TABLE templates_old RENAME TO templates;
