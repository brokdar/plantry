-- +goose Up
CREATE TABLE variant_groups (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE components (
    id                 INTEGER PRIMARY KEY,
    name               TEXT NOT NULL UNIQUE,
    role               TEXT NOT NULL CHECK (role IN ('main','side_starch','side_veg','side_protein','sauce','drink','dessert','standalone')),
    variant_group_id   INTEGER REFERENCES variant_groups(id) ON DELETE SET NULL,
    reference_portions REAL NOT NULL DEFAULT 1 CHECK (reference_portions > 0),
    prep_minutes       INTEGER NOT NULL DEFAULT 0,
    cook_minutes       INTEGER NOT NULL DEFAULT 0,
    image_path         TEXT,
    notes              TEXT,
    last_cooked_at     TEXT,
    cook_count         INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX ix_components_role          ON components(role);
CREATE INDEX ix_components_variant_group ON components(variant_group_id);
CREATE INDEX ix_components_last_cooked   ON components(last_cooked_at);

CREATE TABLE component_ingredients (
    id            INTEGER PRIMARY KEY,
    component_id  INTEGER NOT NULL REFERENCES components(id) ON DELETE CASCADE,
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE RESTRICT,
    amount        REAL NOT NULL CHECK (amount > 0),
    unit          TEXT NOT NULL,
    grams         REAL NOT NULL CHECK (grams > 0),
    sort_order    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX ix_component_ingredients_component ON component_ingredients(component_id);

CREATE TABLE component_instructions (
    id           INTEGER PRIMARY KEY,
    component_id INTEGER NOT NULL REFERENCES components(id) ON DELETE CASCADE,
    step_number  INTEGER NOT NULL,
    text         TEXT NOT NULL
);

CREATE INDEX ix_component_instructions_component ON component_instructions(component_id);

CREATE TABLE component_tags (
    component_id INTEGER NOT NULL REFERENCES components(id) ON DELETE CASCADE,
    tag          TEXT NOT NULL,
    PRIMARY KEY (component_id, tag)
);

CREATE VIRTUAL TABLE components_fts USING fts5(
    name, content='components', content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

-- +goose StatementBegin
CREATE TRIGGER components_ai AFTER INSERT ON components BEGIN
    INSERT INTO components_fts(rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER components_ad AFTER DELETE ON components BEGIN
    INSERT INTO components_fts(components_fts, rowid, name) VALUES ('delete', old.id, old.name);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER components_au AFTER UPDATE ON components BEGIN
    INSERT INTO components_fts(components_fts, rowid, name) VALUES ('delete', old.id, old.name);
    INSERT INTO components_fts(rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS components_au;
DROP TRIGGER IF EXISTS components_ad;
DROP TRIGGER IF EXISTS components_ai;
DROP TABLE IF EXISTS components_fts;
DROP TABLE IF EXISTS component_tags;
DROP TABLE IF EXISTS component_instructions;
DROP TABLE IF EXISTS component_ingredients;
DROP TABLE IF EXISTS components;
DROP TABLE IF EXISTS variant_groups;
