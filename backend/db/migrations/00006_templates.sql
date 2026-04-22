-- +goose Up
CREATE TABLE templates (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE template_components (
    id           INTEGER PRIMARY KEY,
    template_id  INTEGER NOT NULL REFERENCES templates(id)  ON DELETE CASCADE,
    component_id INTEGER NOT NULL REFERENCES components(id) ON DELETE RESTRICT,
    portions     REAL    NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX ix_template_components_template ON template_components(template_id);

-- +goose Down
DROP INDEX IF EXISTS ix_template_components_template;
DROP TABLE IF EXISTS template_components;
DROP TABLE IF EXISTS templates;
