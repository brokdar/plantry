-- +goose Up
-- Unified Food aggregate: collapses ingredients + components into a single
-- recursive entity. LEAF foods carry direct per-100g nutrition; COMPOSED foods
-- aggregate nutrition from child foods via food_components.

CREATE TABLE foods (
    id                 INTEGER PRIMARY KEY,
    name               TEXT NOT NULL,
    kind               TEXT NOT NULL CHECK (kind IN ('leaf','composed')),
    role               TEXT          CHECK (role IN ('main','side_starch','side_veg','side_protein','sauce','drink','dessert','standalone')),

    source             TEXT          CHECK (source IN ('manual','off','fdc')),
    barcode            TEXT,
    off_id             TEXT,
    fdc_id             TEXT,

    kcal_100g          REAL,
    protein_100g       REAL,
    fat_100g           REAL,
    carbs_100g         REAL,
    fiber_100g         REAL,
    sodium_100g        REAL,
    saturated_fat_100g REAL,
    trans_fat_100g     REAL,
    cholesterol_100g   REAL,
    sugar_100g         REAL,
    potassium_100g     REAL,
    calcium_100g       REAL,
    iron_100g          REAL,
    magnesium_100g     REAL,
    phosphorus_100g    REAL,
    zinc_100g          REAL,
    vitamin_a_100g     REAL,
    vitamin_c_100g     REAL,
    vitamin_d_100g     REAL,
    vitamin_b12_100g   REAL,
    vitamin_b6_100g    REAL,
    folate_100g        REAL,

    variant_group_id   INTEGER REFERENCES variant_groups(id) ON DELETE SET NULL,
    reference_portions REAL    CHECK (reference_portions IS NULL OR reference_portions > 0),
    prep_minutes       INTEGER,
    cook_minutes       INTEGER,
    notes              TEXT,

    image_path         TEXT,
    favorite           INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),
    last_cooked_at     TEXT,
    cook_count         INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at         TEXT NOT NULL DEFAULT (datetime('now')),

    -- Kind-specific invariants: LEAF must have source, COMPOSED must have reference_portions.
    CHECK ((kind = 'leaf'     AND source IS NOT NULL AND reference_portions IS NULL) OR
           (kind = 'composed' AND source IS NULL     AND reference_portions IS NOT NULL)),
    -- variant_group_id only valid on COMPOSED foods.
    CHECK (variant_group_id IS NULL OR kind = 'composed')
);

CREATE INDEX ix_foods_kind           ON foods(kind);
CREATE INDEX ix_foods_role           ON foods(role);
CREATE INDEX ix_foods_variant_group  ON foods(variant_group_id);
CREATE INDEX ix_foods_favorite       ON foods(favorite) WHERE favorite = 1;
CREATE INDEX ix_foods_last_cooked    ON foods(last_cooked_at);
CREATE INDEX ix_foods_barcode        ON foods(barcode) WHERE barcode IS NOT NULL;
CREATE INDEX ix_foods_fdc_id         ON foods(fdc_id)  WHERE fdc_id  IS NOT NULL;
CREATE UNIQUE INDEX ux_foods_name_leaf ON foods(name) WHERE kind = 'leaf';

CREATE TABLE food_components (
    id           INTEGER PRIMARY KEY,
    parent_id    INTEGER NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    child_id     INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    amount       REAL NOT NULL CHECK (amount > 0),
    unit         TEXT NOT NULL,
    grams        REAL NOT NULL CHECK (grams > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0,
    CHECK (parent_id != child_id)
);
CREATE INDEX ix_food_components_parent ON food_components(parent_id);
CREATE INDEX ix_food_components_child  ON food_components(child_id);

CREATE TABLE food_instructions (
    id          INTEGER PRIMARY KEY,
    food_id     INTEGER NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    step_number INTEGER NOT NULL,
    text        TEXT NOT NULL
);
CREATE INDEX ix_food_instructions_food ON food_instructions(food_id);

CREATE TABLE food_tags (
    food_id INTEGER NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    tag     TEXT NOT NULL,
    PRIMARY KEY (food_id, tag)
);

CREATE TABLE food_portions (
    food_id INTEGER NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    unit    TEXT NOT NULL,
    grams   REAL NOT NULL CHECK (grams > 0),
    PRIMARY KEY (food_id, unit)
);

CREATE VIRTUAL TABLE foods_fts USING fts5(
    name, content='foods', content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

-- +goose StatementBegin
CREATE TRIGGER foods_ai AFTER INSERT ON foods BEGIN
    INSERT INTO foods_fts(rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER foods_ad AFTER DELETE ON foods BEGIN
    INSERT INTO foods_fts(foods_fts, rowid, name) VALUES ('delete', old.id, old.name);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER foods_au AFTER UPDATE ON foods BEGIN
    INSERT INTO foods_fts(foods_fts, rowid, name) VALUES ('delete', old.id, old.name);
    INSERT INTO foods_fts(rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

-- ──────────────────────────────────────────────────────────────────────
-- Data copy: ingredients → leaf foods, components → composed foods.
-- Triggers above populate foods_fts automatically on each INSERT.
-- ──────────────────────────────────────────────────────────────────────

INSERT INTO foods (
    name, kind, source, barcode, off_id, fdc_id, image_path,
    kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g,
    saturated_fat_100g, trans_fat_100g, cholesterol_100g, sugar_100g,
    potassium_100g, calcium_100g, iron_100g, magnesium_100g, phosphorus_100g, zinc_100g,
    vitamin_a_100g, vitamin_c_100g, vitamin_d_100g, vitamin_b12_100g, vitamin_b6_100g, folate_100g,
    created_at, updated_at
)
SELECT
    name, 'leaf', source, barcode, off_id, fdc_id, image_path,
    kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g,
    saturated_fat_100g, trans_fat_100g, cholesterol_100g, sugar_100g,
    potassium_100g, calcium_100g, iron_100g, magnesium_100g, phosphorus_100g, zinc_100g,
    vitamin_a_100g, vitamin_c_100g, vitamin_d_100g, vitamin_b12_100g, vitamin_b6_100g, folate_100g,
    created_at, updated_at
FROM ingredients
ORDER BY id;

CREATE TEMP TABLE ing_map AS
SELECT i.id AS old_id, f.id AS new_id
FROM ingredients i
JOIN foods f ON f.name = i.name AND f.kind = 'leaf';

INSERT INTO foods (
    name, kind, role, variant_group_id, reference_portions,
    prep_minutes, cook_minutes, image_path, notes,
    last_cooked_at, cook_count, favorite, created_at, updated_at
)
SELECT
    name, 'composed', role, variant_group_id, reference_portions,
    prep_minutes, cook_minutes, image_path, notes,
    last_cooked_at, cook_count, favorite, created_at, updated_at
FROM components
ORDER BY id;

-- Components may share names (variants). Match by (name, creation-order rank).
CREATE TEMP TABLE comp_map AS
WITH ranked_comp AS (
    SELECT id, name, row_number() OVER (PARTITION BY name ORDER BY id) AS rn
    FROM components
),
ranked_food AS (
    SELECT id, name, row_number() OVER (PARTITION BY name ORDER BY id) AS rn
    FROM foods WHERE kind = 'composed'
)
SELECT rc.id AS old_id, rf.id AS new_id
FROM ranked_comp rc
JOIN ranked_food rf ON rf.name = rc.name AND rf.rn = rc.rn;

INSERT INTO food_components (parent_id, child_id, amount, unit, grams, sort_order)
SELECT cm.new_id, im.new_id, ci.amount, ci.unit, ci.grams, ci.sort_order
FROM component_ingredients ci
JOIN comp_map cm ON cm.old_id = ci.component_id
JOIN ing_map  im ON im.old_id = ci.ingredient_id;

INSERT INTO food_instructions (food_id, step_number, text)
SELECT cm.new_id, ci.step_number, ci.text
FROM component_instructions ci
JOIN comp_map cm ON cm.old_id = ci.component_id;

INSERT INTO food_tags (food_id, tag)
SELECT cm.new_id, ct.tag
FROM component_tags ct
JOIN comp_map cm ON cm.old_id = ct.component_id;

INSERT INTO food_portions (food_id, unit, grams)
SELECT im.new_id, ip.unit, ip.grams
FROM ingredient_portions ip
JOIN ing_map im ON im.old_id = ip.ingredient_id;

-- ──────────────────────────────────────────────────────────────────────
-- Rewire plate_components and template_components from component_id → food_id.
-- ──────────────────────────────────────────────────────────────────────

CREATE TABLE plate_components_new (
    id         INTEGER PRIMARY KEY,
    plate_id   INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    food_id    INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    portions   REAL NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order INTEGER NOT NULL DEFAULT 0
);

INSERT INTO plate_components_new (id, plate_id, food_id, portions, sort_order)
SELECT pc.id, pc.plate_id, cm.new_id, pc.portions, pc.sort_order
FROM plate_components pc
JOIN comp_map cm ON cm.old_id = pc.component_id;

DROP INDEX IF EXISTS ix_plate_components_plate;
DROP TABLE plate_components;
ALTER TABLE plate_components_new RENAME TO plate_components;
CREATE INDEX ix_plate_components_plate ON plate_components(plate_id);

CREATE TABLE template_components_new (
    id          INTEGER PRIMARY KEY,
    template_id INTEGER NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    food_id     INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    portions    REAL    NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order  INTEGER NOT NULL DEFAULT 0
);

INSERT INTO template_components_new (id, template_id, food_id, portions, sort_order)
SELECT tc.id, tc.template_id, cm.new_id, tc.portions, tc.sort_order
FROM template_components tc
JOIN comp_map cm ON cm.old_id = tc.component_id;

DROP INDEX IF EXISTS ix_template_components_template;
DROP TABLE template_components;
ALTER TABLE template_components_new RENAME TO template_components;
CREATE INDEX ix_template_components_template ON template_components(template_id);

-- ──────────────────────────────────────────────────────────────────────
-- Drop the old aggregates. variant_groups stays (foods.variant_group_id still references it).
-- ──────────────────────────────────────────────────────────────────────

DROP TRIGGER IF EXISTS components_au;
DROP TRIGGER IF EXISTS components_ad;
DROP TRIGGER IF EXISTS components_ai;
DROP TABLE IF EXISTS components_fts;
DROP TABLE IF EXISTS component_tags;
DROP TABLE IF EXISTS component_instructions;
DROP TABLE IF EXISTS component_ingredients;
DROP TABLE IF EXISTS components;

DROP TABLE IF EXISTS ingredient_portions;

DROP TRIGGER IF EXISTS ingredients_au;
DROP TRIGGER IF EXISTS ingredients_ad;
DROP TRIGGER IF EXISTS ingredients_ai;
DROP TABLE IF EXISTS ingredients_fts;
DROP TABLE IF EXISTS ingredients;

DROP TABLE ing_map;
DROP TABLE comp_map;

-- +goose Down
-- Reverses the schema; does NOT round-trip data. Restore the pre-migration
-- SQLite backup if you need pre-13 data. Dev-only migration.

DROP INDEX IF EXISTS ix_template_components_template;
DROP TABLE IF EXISTS template_components;
CREATE TABLE template_components (
    id           INTEGER PRIMARY KEY,
    template_id  INTEGER NOT NULL REFERENCES templates(id)  ON DELETE CASCADE,
    component_id INTEGER NOT NULL REFERENCES components(id) ON DELETE RESTRICT,
    portions     REAL    NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_template_components_template ON template_components(template_id);

DROP INDEX IF EXISTS ix_plate_components_plate;
DROP TABLE IF EXISTS plate_components;
CREATE TABLE plate_components (
    id           INTEGER PRIMARY KEY,
    plate_id     INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    component_id INTEGER NOT NULL REFERENCES components(id) ON DELETE RESTRICT,
    portions     REAL NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_plate_components_plate ON plate_components(plate_id);

DROP TRIGGER IF EXISTS foods_au;
DROP TRIGGER IF EXISTS foods_ad;
DROP TRIGGER IF EXISTS foods_ai;
DROP TABLE IF EXISTS foods_fts;
DROP TABLE IF EXISTS food_portions;
DROP TABLE IF EXISTS food_tags;
DROP TABLE IF EXISTS food_instructions;
DROP TABLE IF EXISTS food_components;
DROP TABLE IF EXISTS foods;

CREATE TABLE ingredients (
    id              INTEGER PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    source          TEXT NOT NULL CHECK (source IN ('manual','off','fdc')),
    barcode         TEXT,
    off_id          TEXT,
    fdc_id          TEXT,
    image_path      TEXT,
    kcal_100g       REAL NOT NULL DEFAULT 0,
    protein_100g    REAL NOT NULL DEFAULT 0,
    fat_100g        REAL NOT NULL DEFAULT 0,
    carbs_100g      REAL NOT NULL DEFAULT 0,
    fiber_100g      REAL NOT NULL DEFAULT 0,
    sodium_100g     REAL NOT NULL DEFAULT 0,
    saturated_fat_100g REAL,
    trans_fat_100g  REAL,
    cholesterol_100g REAL,
    sugar_100g      REAL,
    potassium_100g  REAL,
    calcium_100g    REAL,
    iron_100g       REAL,
    magnesium_100g  REAL,
    phosphorus_100g REAL,
    zinc_100g       REAL,
    vitamin_a_100g  REAL,
    vitamin_c_100g  REAL,
    vitamin_d_100g  REAL,
    vitamin_b12_100g REAL,
    vitamin_b6_100g REAL,
    folate_100g     REAL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX ix_ingredients_barcode ON ingredients(barcode);
CREATE INDEX ix_ingredients_fdc_id  ON ingredients(fdc_id);
CREATE VIRTUAL TABLE ingredients_fts USING fts5(
    name, content='ingredients', content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

-- +goose StatementBegin
CREATE TRIGGER ingredients_ai AFTER INSERT ON ingredients BEGIN
    INSERT INTO ingredients_fts(rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER ingredients_ad AFTER DELETE ON ingredients BEGIN
    INSERT INTO ingredients_fts(ingredients_fts, rowid, name) VALUES ('delete', old.id, old.name);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER ingredients_au AFTER UPDATE ON ingredients BEGIN
    INSERT INTO ingredients_fts(ingredients_fts, rowid, name) VALUES ('delete', old.id, old.name);
    INSERT INTO ingredients_fts(rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

CREATE TABLE ingredient_portions (
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    unit          TEXT NOT NULL,
    grams         REAL NOT NULL CHECK (grams > 0),
    PRIMARY KEY (ingredient_id, unit)
);

CREATE TABLE components (
    id                 INTEGER PRIMARY KEY,
    name               TEXT NOT NULL,
    role               TEXT NOT NULL CHECK (role IN ('main','side_starch','side_veg','side_protein','sauce','drink','dessert','standalone')),
    variant_group_id   INTEGER REFERENCES variant_groups(id) ON DELETE SET NULL,
    reference_portions REAL NOT NULL DEFAULT 1 CHECK (reference_portions > 0),
    prep_minutes       INTEGER,
    cook_minutes       INTEGER,
    image_path         TEXT,
    notes              TEXT,
    last_cooked_at     TEXT,
    cook_count         INTEGER NOT NULL DEFAULT 0,
    favorite           INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX ix_components_role          ON components(role);
CREATE INDEX ix_components_variant_group ON components(variant_group_id);
CREATE INDEX ix_components_last_cooked   ON components(last_cooked_at);
CREATE INDEX ix_components_favorite      ON components(favorite) WHERE favorite = 1;

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
