-- +goose Up
-- Add day_offset to template_components using a table rebuild (SQLite column add limitation)
CREATE TABLE template_components_new (
    id           INTEGER PRIMARY KEY,
    template_id  INTEGER NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    food_id      INTEGER NOT NULL REFERENCES foods(id)    ON DELETE RESTRICT,
    portions     REAL    NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0,
    day_offset   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO template_components_new (id, template_id, food_id, portions, sort_order, day_offset)
    SELECT id, template_id, food_id, portions, sort_order, 0 FROM template_components;
DROP INDEX IF EXISTS ix_template_components_template;
DROP TABLE template_components;
ALTER TABLE template_components_new RENAME TO template_components;
CREATE INDEX ix_template_components_template ON template_components(template_id);

-- +goose Down
CREATE TABLE template_components_old (
    id           INTEGER PRIMARY KEY,
    template_id  INTEGER NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    food_id      INTEGER NOT NULL REFERENCES foods(id)    ON DELETE RESTRICT,
    portions     REAL    NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO template_components_old (id, template_id, food_id, portions, sort_order)
    SELECT id, template_id, food_id, portions, sort_order FROM template_components;
DROP INDEX IF EXISTS ix_template_components_template;
DROP TABLE template_components;
ALTER TABLE template_components_old RENAME TO template_components;
CREATE INDEX ix_template_components_template ON template_components(template_id);
