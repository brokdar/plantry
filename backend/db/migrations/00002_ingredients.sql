-- +goose Up
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

-- +goose Down
DROP TRIGGER IF EXISTS ingredients_au;
DROP TRIGGER IF EXISTS ingredients_ad;
DROP TRIGGER IF EXISTS ingredients_ai;
DROP TABLE IF EXISTS ingredients_fts;
DROP TABLE IF EXISTS ingredients;
