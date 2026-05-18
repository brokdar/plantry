-- +goose Up
-- +goose StatementBegin
-- Plate components grow a kind-aware quantity model. Composed foods carry an
-- INTEGER portions count; leaf foods carry (amount, unit, grams, grams_source)
-- with grams resolved server-side. Old REAL portions rows are folded into the
-- new shape: composed plate_components round portions to the nearest integer
-- (half-up — most existing rows are 1.0 / 2.0); leaf rows backfill to
-- (amount = portions * 100, unit = 'g', grams = portions * 100,
-- grams_source = 'direct'), matching how the old "× 1.0" really meant "100 g".
--
-- Implementation note: SQLite cannot drop a CHECK constraint or change a column
-- type in place, so we rebuild the table via the standard new-table + copy +
-- rename pattern (same as 00013, 00014). The whole migration runs inside
-- goose's per-statement transaction.

CREATE TABLE plate_components_new (
    id            INTEGER PRIMARY KEY,
    plate_id      INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    food_id       INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    portions      INTEGER,
    amount        REAL,
    unit          TEXT,
    grams         REAL,
    grams_source  TEXT,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    -- Exactly one quantity shape: composed (portions only) XOR leaf
    -- (amount + unit + grams). Never both, never neither.
    CHECK (
        (portions IS NOT NULL AND portions > 0
            AND amount IS NULL AND unit IS NULL AND grams IS NULL AND grams_source IS NULL)
        OR
        (portions IS NULL
            AND amount IS NOT NULL AND amount > 0
            AND unit IS NOT NULL AND unit != ''
            AND grams IS NOT NULL AND grams >= 0)
    )
);

-- Backfill: branch on the food's kind.
--   composed → keep portions, rounded half-up to an integer (CAST(x + 0.5 AS INTEGER))
--   leaf     → amount = portions * 100, unit = 'g', grams = portions * 100, source = 'direct'
INSERT INTO plate_components_new (
    id, plate_id, food_id, portions, amount, unit, grams, grams_source, sort_order
)
SELECT
    pc.id,
    pc.plate_id,
    pc.food_id,
    CASE WHEN f.kind = 'composed'
         THEN MAX(1, CAST(pc.portions + 0.5 AS INTEGER))
         ELSE NULL
    END AS portions,
    CASE WHEN f.kind = 'leaf' THEN pc.portions * 100.0 ELSE NULL END AS amount,
    CASE WHEN f.kind = 'leaf' THEN 'g' ELSE NULL END AS unit,
    CASE WHEN f.kind = 'leaf' THEN pc.portions * 100.0 ELSE NULL END AS grams,
    CASE WHEN f.kind = 'leaf' THEN 'direct' ELSE NULL END AS grams_source,
    pc.sort_order
FROM plate_components pc
JOIN foods f ON f.id = pc.food_id;

DROP INDEX IF EXISTS ix_plate_components_plate;
DROP TABLE plate_components;
ALTER TABLE plate_components_new RENAME TO plate_components;
CREATE INDEX ix_plate_components_plate ON plate_components(plate_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Reverse: collapse the kind-aware quantity columns back into a single REAL
-- portions column. For leaf rows, portions = grams / 100 (the inverse of the
-- up-direction backfill). Composed rows already store INTEGER portions; cast
-- to REAL on copy.
CREATE TABLE plate_components_old (
    id         INTEGER PRIMARY KEY,
    plate_id   INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    food_id    INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    portions   REAL NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order INTEGER NOT NULL DEFAULT 0
);

INSERT INTO plate_components_old (id, plate_id, food_id, portions, sort_order)
SELECT
    id,
    plate_id,
    food_id,
    CASE
        WHEN portions IS NOT NULL THEN CAST(portions AS REAL)
        WHEN grams IS NOT NULL AND grams > 0 THEN grams / 100.0
        ELSE 1.0
    END,
    sort_order
FROM plate_components;

DROP INDEX IF EXISTS ix_plate_components_plate;
DROP TABLE plate_components;
ALTER TABLE plate_components_old RENAME TO plate_components;
CREATE INDEX ix_plate_components_plate ON plate_components(plate_id);
-- +goose StatementEnd
