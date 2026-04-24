# Plan: Unified `Food` Domain Refactor

**Status:** Draft — awaiting approval to start
**Owner:** TBD
**Target branch:** `feat/unified-food` (off `main`)
**Estimate:** 2–3 days focused work, split into 8 verifiable phases

---

## 1. Executive Summary

Collapse the current two-entity model (`Ingredient` + `Component`) into a single recursive aggregate called `Food`. A `Food` can be a **leaf** (single edible item with direct nutrition — apple, rice, banana) or **composed** (built from child `Food`s with optional instructions — schnitzel with potato salad, homemade pesto). Plates and templates reference one thing: a `Food`.

This removes the asymmetry where ingredients cannot be placed directly into a plate slot and requires users to wrap trivial items like "apple" in a dummy component. It also unlocks recursive composition (sub-recipes, batch-cooked leftovers as ingredients of later meals) at no extra modeling cost.

The refactor is a one-time migration. It touches every layer but is mechanical: schema → sqlc → domain → handlers → frontend API → UI. Each phase ends with a green test suite before the next begins.

---

## 2. Goals and Non-Goals

### Goals

- Single aggregate (`Food`) underlies ingredients, recipes, sides, snacks, sub-recipes, and leftovers.
- Plates and templates reference `food_id`. No polymorphism.
- Nutrition resolves uniformly via one recursive walk.
- Shopping list still aggregates to shoppable leaf foods with gram totals.
- No loss of existing data: every current `ingredient` and `component` survives migration with identical behavior.
- Idempotent, reversible migration: a down-migration restores the prior schema from the `foods` table without data loss.

### Non-Goals (deferred)

- Redesigning the UI of the component library / ingredient catalog (two list views will remain initially, filtered by `kind`; unification of the browse UX is a later design task).
- New features (photos per food, bulk edit, export). Parity first.
- Changing AI/import flows. They will be rewired to produce `Food` records but not rethought.
- Reworking `variant_groups` semantics. It moves to `foods.variant_group_id` unchanged.
- Deleting the `ingredient_portions` table concept. Portions move to `food_portions` with identical structure.

---

## 3. Current State

### Tables involved (source: `backend/db/migrations/`)

| Table | Role |
|---|---|
| `ingredients` | Nutritional atoms; 22 columns including 16 extended nutrients. |
| `ingredient_portions` | Per-ingredient unit→grams overrides. |
| `ingredients_fts` | FTS5 virtual table with insert/delete/update triggers. |
| `components` | Dishes/recipes; has role, variant_group_id, reference_portions, cook tracking, favorite. |
| `component_ingredients` | M:N link `component → ingredient` with amount, unit, grams, sort_order. |
| `component_instructions` | Ordered cooking steps. |
| `component_tags` | (component_id, tag) PK. |
| `components_fts` | FTS5 virtual table + triggers. |
| `variant_groups` | Groups variants of a component (e.g., normal vs. low-cal). |
| `plates` + `plate_components` | Scheduled meals; `plate_components.component_id` is the single link. |
| `templates` + `template_components` | Reusable plate compositions; `template_components.component_id` is the single link. |

### Domain packages

- `domain/ingredient` — aggregate, repo interface, service, AI resolver, portion provider.
- `domain/component` — aggregate, repo interface, service (with grams resolver).
- `domain/plate` — aggregate with `PlateComponent` referencing `component_id`.
- `domain/template` — template + `TemplateComponent`.
- `domain/shopping` — aggregator walks `plates[] → refs{component_id → ingredients}`.
- `domain/nutrition` — `FromIngredients` + `PerPortion` for flat (non-recursive) components.
- `domain/planner`, `domain/slot` — consume plates.

### HTTP handlers

Each aggregate has its own file; `ingredients.go`, `components.go`, `plates.go`, `templates.go`, `portions.go`, `lookup.go`.

### Frontend

- `lib/api/{ingredients,components,plates,templates,portions,lookup}.ts`
- `lib/queries/{ingredients,components,plates,templates,plate-patches,slots,weeks}.ts`
- `lib/schemas/{ingredient,component,plate,template}.ts`
- Routes under `frontend/src/routes/` (file-based; heavy use across ingredient/component/planner screens).

---

## 4. Target Model

### Schema (new tables)

```sql
CREATE TABLE foods (
    id                 INTEGER PRIMARY KEY,
    name               TEXT NOT NULL,
    kind               TEXT NOT NULL CHECK (kind IN ('leaf','composed')),
    role               TEXT          CHECK (role IN ('main','side_starch','side_veg','side_protein','sauce','drink','dessert','standalone')),

    -- LEAF provenance (nullable for composed)
    source             TEXT          CHECK (source IN ('manual','off','fdc')),
    barcode            TEXT,
    off_id             TEXT,
    fdc_id             TEXT,

    -- Direct nutrition per 100g (LEAF sets these; COMPOSED leaves them NULL and is aggregated from children)
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

    -- COMPOSED metadata (nullable for leaf)
    variant_group_id   INTEGER REFERENCES variant_groups(id) ON DELETE SET NULL,
    reference_portions REAL    CHECK (reference_portions IS NULL OR reference_portions > 0),
    prep_minutes       INTEGER,
    cook_minutes       INTEGER,
    notes              TEXT,

    -- Shared
    image_path         TEXT,
    favorite           INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),
    last_cooked_at     TEXT,
    cook_count         INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX ix_foods_kind            ON foods(kind);
CREATE INDEX ix_foods_role            ON foods(role);
CREATE INDEX ix_foods_variant_group   ON foods(variant_group_id);
CREATE INDEX ix_foods_favorite        ON foods(favorite) WHERE favorite = 1;
CREATE INDEX ix_foods_last_cooked     ON foods(last_cooked_at);
CREATE INDEX ix_foods_barcode         ON foods(barcode) WHERE barcode IS NOT NULL;
CREATE INDEX ix_foods_fdc_id          ON foods(fdc_id)  WHERE fdc_id  IS NOT NULL;
CREATE UNIQUE INDEX ux_foods_name_leaf ON foods(name) WHERE kind = 'leaf';
-- Leaf uniqueness preserves old ingredients UNIQUE(name). Composed foods may share names across variants.

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
-- Plus the standard ai/ad/au triggers (pattern from existing FTS).
```

### Retargeted references

- `plate_components.component_id` → rename column to `food_id` (FK `foods.id ON DELETE RESTRICT`).
- `template_components.component_id` → rename to `food_id` (same behavior).

### Cycle prevention

`food_components.parent_id != child_id` alone blocks self-loops. Multi-step cycles (A→B→A) are prevented in the domain service on write via a DFS reachability check before inserting. We do not attempt to enforce this in SQL (would require recursive CTE triggers, not worth the complexity).

### Kind invariants (enforced in domain service)

- `kind = 'leaf'` ⇒ no `food_components` children, no `food_instructions`, `reference_portions` NULL, `source` NOT NULL, nutrition columns SHOULD be set.
- `kind = 'composed'` ⇒ at least one child, `reference_portions` NOT NULL and > 0, `source` NULL, direct nutrition columns NULL (nutrition is aggregated).

DB CHECKs enforce the nullability rules; quantity rules (e.g., "composed must have ≥ 1 child") are enforced in the domain service, since SQL cannot express "exists" cleanly in a CHECK.

### Nutrition resolution

- LEAF: read direct columns.
- COMPOSED: walk `food_components`, for each child recursively resolve per-100g macros, multiply by `child.grams / 100`, sum. Divide total by `reference_portions` for per-portion output.
- Cache resolution per food_id within a single HTTP request (simple in-memory map). Not persisted; always recomputed to avoid staleness.

---

## 5. Domain Model (Go)

Replace `domain/ingredient` and `domain/component` with `domain/food`:

```go
package food

type Kind string

const (
    KindLeaf     Kind = "leaf"
    KindComposed Kind = "composed"
)

type Role string // unchanged values; only valid for COMPOSED or optional for LEAF

type Food struct {
    ID       int64
    Name     string
    Kind     Kind
    Role     *Role

    // Leaf-only provenance
    Source    *string
    Barcode   *string
    OffID     *string
    FdcID     *string

    // Leaf-only direct nutrition (all pointers; COMPOSED keeps these nil)
    Kcal100g    *float64
    Protein100g *float64
    // ... (all 22 nutrient fields)

    // Composed-only metadata
    VariantGroupID    *int64
    ReferencePortions *float64
    PrepMinutes       *int
    CookMinutes       *int
    Notes             *string

    // Shared
    ImagePath    *string
    Favorite     bool
    LastCookedAt *time.Time
    CookCount    int

    Children     []FoodComponent
    Instructions []Instruction
    Tags         []string
    Portions     []Portion

    CreatedAt time.Time
    UpdatedAt time.Time
}

type FoodComponent struct {
    ID        int64
    ParentID  int64
    ChildID   int64
    ChildName string // populated on read
    ChildKind Kind   // populated on read — UI can show "🍎 apple (leaf)" vs "🍛 curry (composed)"
    Amount    float64
    Unit      string
    Grams     float64
    GramsSource string // same set as today (direct/portion/default/fallback/manual)
    SortOrder int
}
```

Services:

- `food.Service` — replaces both `ingredient.Service` and `component.Service`. Validation, grams resolution, cycle detection, create/update/delete with kind checks.
- `nutrition.Resolver` — new; walks food tree.
- `plate.Service`, `template.Service` — updated to reference food IDs; no other structural change.
- `shopping.FromPlates` — updated to walk food tree and collect LEAF children with their aggregated grams.

Kept as-is:

- `domain/slot`, `domain/planner`, `domain/profile`, `domain/feedback`, `domain/settings`, `domain/agent`, `domain/importer` (but the importer writes `Food` records instead of `Ingredient`+`Component`).

---

## 6. Migration Strategy

### Principles

1. **One Goose migration** (`00013_unified_food.sql`) creates new tables + copies data + rewires FKs + drops old tables, inside a single transaction for atomicity.
2. **Preserve all IDs where possible** by reusing ingredient + component IDs in a single ID space. Since both came from separate sequences, we re-ID on the way in and keep a mapping table that survives the migration for debuggability.
3. **Data copy with invariants verified pre-commit** via a sequence of `SELECT` assertions in the same transaction (via `goose` statement blocks). If any invariant fails, the transaction rolls back.
4. **Down migration** recreates old tables from the new ones by splitting `foods` on `kind`. Data round-trips losslessly because no information is merged that wasn't originally separate.

### Data copy (inside migration)

```sql
-- 1. Create foods + food_* tables as above.
-- 2. Copy ingredients → foods (LEAF)
INSERT INTO foods (name, kind, source, barcode, off_id, fdc_id, image_path,
                   kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g,
                   saturated_fat_100g, trans_fat_100g, cholesterol_100g, sugar_100g,
                   potassium_100g, calcium_100g, iron_100g, magnesium_100g, phosphorus_100g, zinc_100g,
                   vitamin_a_100g, vitamin_c_100g, vitamin_d_100g, vitamin_b12_100g, vitamin_b6_100g, folate_100g,
                   created_at, updated_at)
SELECT name, 'leaf', source, barcode, off_id, fdc_id, image_path,
       kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g,
       saturated_fat_100g, trans_fat_100g, cholesterol_100g, sugar_100g,
       potassium_100g, calcium_100g, iron_100g, magnesium_100g, phosphorus_100g, zinc_100g,
       vitamin_a_100g, vitamin_c_100g, vitamin_d_100g, vitamin_b12_100g, vitamin_b6_100g, folate_100g,
       created_at, updated_at
FROM ingredients;

-- Keep a mapping so we can remap children after.
CREATE TEMP TABLE ing_map AS
SELECT i.id AS old_id, f.id AS new_id
FROM ingredients i JOIN foods f ON f.name = i.name AND f.kind = 'leaf';

-- 3. Copy components → foods (COMPOSED)
INSERT INTO foods (name, kind, role, variant_group_id, reference_portions, prep_minutes, cook_minutes,
                   image_path, notes, last_cooked_at, cook_count, favorite, created_at, updated_at)
SELECT name, 'composed', role, variant_group_id, reference_portions, prep_minutes, cook_minutes,
       image_path, notes, last_cooked_at, cook_count, favorite, created_at, updated_at
FROM components;

-- Components may share names across variants, so map by (old_id, created_at, name).
-- Use a deterministic matcher: row_number OVER (PARTITION BY name ORDER BY id).
CREATE TEMP TABLE comp_map AS
SELECT c.id AS old_id, f.id AS new_id
FROM components c
JOIN (
    SELECT id, name, row_number() OVER (PARTITION BY name ORDER BY id) AS rn
    FROM foods WHERE kind = 'composed'
) f_ranked ON f_ranked.name = c.name
  AND f_ranked.rn = (SELECT row_number() OVER (PARTITION BY name ORDER BY id)
                     FROM components c2 WHERE c2.id = c.id)
JOIN foods f ON f.id = f_ranked.id;
-- Simpler fallback: insert components one-by-one in a Go pre-migration step if
-- window-function matching proves fragile in SQLite.

-- 4. Copy component_ingredients → food_components
INSERT INTO food_components (parent_id, child_id, amount, unit, grams, sort_order)
SELECT cm.new_id, im.new_id, ci.amount, ci.unit, ci.grams, ci.sort_order
FROM component_ingredients ci
JOIN comp_map cm ON cm.old_id = ci.component_id
JOIN ing_map  im ON im.old_id = ci.ingredient_id;

-- 5. Copy component_instructions → food_instructions
INSERT INTO food_instructions (food_id, step_number, text)
SELECT cm.new_id, ci.step_number, ci.text
FROM component_instructions ci JOIN comp_map cm ON cm.old_id = ci.component_id;

-- 6. Copy component_tags → food_tags
INSERT INTO food_tags (food_id, tag)
SELECT cm.new_id, ct.tag FROM component_tags ct JOIN comp_map cm ON cm.old_id = ct.component_id;

-- 7. Copy ingredient_portions → food_portions
INSERT INTO food_portions (food_id, unit, grams)
SELECT im.new_id, ip.unit, ip.grams
FROM ingredient_portions ip JOIN ing_map im ON im.old_id = ip.ingredient_id;

-- 8. Rebuild FTS
INSERT INTO foods_fts(rowid, name) SELECT id, name FROM foods;

-- 9. Create new plate_components and template_components with food_id, backfill, swap.
CREATE TABLE plate_components_new (
    id         INTEGER PRIMARY KEY,
    plate_id   INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    food_id    INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    portions   REAL NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order INTEGER NOT NULL DEFAULT 0
);
INSERT INTO plate_components_new (id, plate_id, food_id, portions, sort_order)
SELECT pc.id, pc.plate_id, cm.new_id, pc.portions, pc.sort_order
FROM plate_components pc JOIN comp_map cm ON cm.old_id = pc.component_id;
DROP TABLE plate_components;
ALTER TABLE plate_components_new RENAME TO plate_components;
CREATE INDEX ix_plate_components_plate ON plate_components(plate_id);

-- Same pattern for template_components.

-- 10. Drop old tables.
DROP TABLE component_tags;
DROP TABLE component_instructions;
DROP TABLE component_ingredients;
DROP TABLE components_fts;
DROP TRIGGER components_ai; DROP TRIGGER components_ad; DROP TRIGGER components_au;
DROP TABLE components;
DROP TABLE ingredient_portions;
DROP TABLE ingredients_fts;
DROP TRIGGER ingredients_ai; DROP TRIGGER ingredients_ad; DROP TRIGGER ingredients_au;
DROP TABLE ingredients;
```

### Invariant checks inside migration (fail-fast)

After step 8, run:

```sql
-- Every ingredient landed exactly once as a leaf food.
SELECT CASE WHEN (SELECT COUNT(*) FROM ingredients)
              = (SELECT COUNT(*) FROM foods WHERE kind = 'leaf')
         THEN 1 ELSE raise_error('ingredient count mismatch') END;

-- Every component landed exactly once as a composed food.
SELECT CASE WHEN (SELECT COUNT(*) FROM components)
              = (SELECT COUNT(*) FROM foods WHERE kind = 'composed')
         THEN 1 ELSE raise_error('component count mismatch') END;

-- component_ingredients → food_components row count matches.
SELECT CASE WHEN (SELECT COUNT(*) FROM component_ingredients)
              = (SELECT COUNT(*) FROM food_components)
         THEN 1 ELSE raise_error('food_components count mismatch') END;

-- Every parent_id in food_components is a composed food.
SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM food_components fc JOIN foods f ON f.id = fc.parent_id WHERE f.kind != 'composed'
) THEN 1 ELSE raise_error('composed parent invariant violated') END;

-- Sum of grams per component preserved.
SELECT CASE WHEN (SELECT COALESCE(SUM(grams), 0) FROM component_ingredients)
              = (SELECT COALESCE(SUM(grams), 0) FROM food_components)
         THEN 1 ELSE raise_error('grams sum mismatch') END;
```

`raise_error` is not native SQLite; we implement these as Go-side assertions executed by Goose before it commits the transaction (Goose supports Go migrations). **Decision:** use a Go-based migration (`00013_unified_food.go`) with SQL blocks + assertion blocks, not a plain `.sql` migration, so we can run verification inside the same `*sql.Tx`.

### Down migration

Reverse of above: recreate `ingredients`, `components`, `component_*`, `ingredient_portions`, split `foods` on kind, copy rows back using stored id mappings. Requires us to **keep the `comp_map` and `ing_map` temp tables as permanent audit tables** (`_migration_00013_ing_map`, `_migration_00013_comp_map`) so we can round-trip without guessing. These can be dropped in a later cleanup migration after the change is blessed.

---

## 7. Phased Execution

Each phase ends at a **green gate**: `go test ./... -race`, `golangci-lint run`, `bun run check`. Do not start phase N+1 with phase N red.

### Phase 0 — Safety net (½ day)

- Create branch `feat/unified-food` from `main`.
- Back up any local SQLite files used for manual testing.
- Add a golden-data integration test that seeds a known set of ingredients + components + plates against the **current** schema, queries nutrition/shopping/planner endpoints, and snapshots responses. This is the oracle we'll run against the migrated DB too.
- **Gate:** golden test passes on `main`.

### Phase 1 — Schema migration (½ day)

- Write `backend/db/migrations/00013_unified_food.go` (Go migration for assertion support).
- Include the DDL + data copy + invariant checks + FK rewiring.
- Write `backend/internal/adapters/sqlite/migration_00013_test.go` that:
  1. Applies migrations 1–12 on a fresh DB.
  2. Seeds via direct SQL: ~10 ingredients, ~5 components (including one with variant group, one with tags, one with instructions, one with extended nutrients), ~2 plates, ~1 template.
  3. Applies migration 13.
  4. Asserts row counts, gram sums, FK integrity, FTS content.
  5. Applies down migration.
  6. Asserts data matches the pre-13 seed byte-for-byte (modulo `updated_at`).
- **Gate:** migration test passes, including up+down round-trip.

### Phase 2 — sqlc regeneration (½ day)

- Rewrite `queries/ingredients.sql` + `queries/components.sql` into a single `queries/foods.sql`.
- Update `queries/planner.sql` and `queries/templates.sql` to reference `food_id`.
- Run `sqlc generate`.
- Update `backend/internal/adapters/sqlite/{ingredient,component}_repo.go` → merge into `food_repo.go`.
- **Gate:** `go build ./...` succeeds; old repo tests temporarily commented out (restored next phase).

### Phase 3 — Domain refactor (1 day)

- Create `domain/food` with aggregate, repo interface, service (validation, kind invariants, grams resolver, cycle check).
- Delete `domain/ingredient` and `domain/component` after callers are rewired.
- Update `domain/plate`, `domain/template` to use `FoodID`.
- Rewrite `domain/shopping/aggregator.go`: walk food tree to collect leaf children + grams.
- Add `domain/nutrition.Resolver` with recursive walk + per-request memoization.
- Restore & adapt existing unit tests; add:
  - Cycle detection test (attempt to add A→B→A).
  - Leaf invariant test (cannot add children or instructions to leaf).
  - Composed invariant test (must have ≥1 child; `reference_portions` required).
  - Recursive nutrition test (3-level deep food tree).
  - Shopping aggregation with nested composed food referenced by another composed food.
- **Gate:** `go test ./... -race` green; `golangci-lint run` clean.

### Phase 4 — HTTP handlers + routes (½ day)

- Replace `handlers/ingredients.go` + `handlers/components.go` with `handlers/foods.go`, exposing `/api/foods` with `?kind=leaf|composed` filter.
- Keep legacy routes as thin redirects for one release cycle: `GET /api/ingredients` → `GET /api/foods?kind=leaf`, `GET /api/components` → `GET /api/foods?kind=composed`. (Optional; drop immediately if no external consumer.)
- Update `handlers/plates.go`, `handlers/templates.go`, `handlers/lookup.go`, `handlers/portions.go` to use food IDs.
- Update handler tests.
- Run the Phase 0 golden test against the new endpoints; responses should match (shape may differ slightly — update the snapshot and review the diff line-by-line before accepting).
- **Gate:** handler tests green; golden-data integration test passes against new endpoints.

### Phase 5 — Frontend API + schemas (½ day)

- Replace `lib/api/ingredients.ts` + `lib/api/components.ts` with `lib/api/foods.ts`.
- Replace `lib/schemas/{ingredient,component}.ts` with `lib/schemas/food.ts` (discriminated union on `kind`).
- Update `lib/queries/*` and `lib/queries/keys.ts`.
- Update `lib/api/{plates,templates,lookup,portions}.ts` to send `food_id`.
- Update `frontend/src/test/fixtures.ts`.
- **Gate:** `bun run typecheck` clean; `bun run test` green.

### Phase 6 — Frontend UI (½ day)

- Update all route components that render ingredient/component pickers, lists, edit forms. The UI can keep two "browse" tabs (*Lebensmittel* = leaf, *Rezepte* = composed) for familiarity but both feed into the same `foods` endpoint.
- The plate component picker is updated to show both kinds, with a visual distinction (e.g., chip color or icon). This is the payoff: user can drop an apple directly onto a plate.
- Update any copy referring to "component" / "ingredient" to neutral terms where appropriate (out of scope: full i18n rewording — minimal change for parity).
- **Gate:** `bun run check` green. Manual smoke test: create leaf apple → plate it as snack → create composed schnitzel → plate it → view shopping list → assert apple appears.

### Phase 7 — E2E + final validation (½ day)

- Update existing Playwright e2e to use new API/UI.
- Add new e2e: "place leaf food directly onto plate" as a named test.
- Run `--repeat-each=10 --workers=4` on the full e2e suite — must be zero-failure before merge.
- Verify `go test -race ./...` on a fresh DB migrated from empty, and on a copy of a dev DB migrated from realistic data.
- **Gate:** all suites green; manual walk-through of planner, ingredient library, recipe library, shopping list.

---

## 8. Validation Strategy

### Migration correctness (DB-level)

Enforced inside `migration_00013_test.go`:

1. **Row count preservation**: `ingredients` → `foods WHERE kind='leaf'`, `components` → `foods WHERE kind='composed'`, `component_ingredients` → `food_components`, `component_instructions` → `food_instructions`, `component_tags` → `food_tags`, `ingredient_portions` → `food_portions`.
2. **Sum preservation**: `SUM(grams)` across `component_ingredients` vs. `food_components`; per-ingredient nutrition sums.
3. **Reference integrity**: every `plate_components.food_id` resolves; every `template_components.food_id` resolves; every `food_components.parent_id` is a composed food; every `food_components.child_id` exists.
4. **FTS content**: `SELECT rowid, name FROM foods_fts` matches `SELECT id, name FROM foods`.
5. **Cycle-free**: no cycles in `food_components` (recursive CTE check).
6. **Round-trip**: apply down migration, compare to pre-13 snapshot.

### Behavioral correctness (end-to-end)

1. **Golden snapshot** from Phase 0 (pre-migration) must match post-migration responses for:
   - `GET /api/components/:id` (now served via `/api/foods/:id` with kind=composed) — nutrition per portion, ingredients list, instructions.
   - `GET /api/plates/:id` — components list, nutrition totals.
   - `GET /api/weeks/:id/shopping` — aggregated ingredient grams.
2. **New behavioral coverage**:
   - Drop a leaf food onto a plate → shopping list shows the leaf directly.
   - Create a composed food that uses another composed food as a child (e.g., "pizza" referencing "homemade dough") → nutrition rolls up correctly through 2 levels; shopping list resolves to the underlying leaves.
   - Attempt to create a cycle → rejected with clear error.
   - Attempt to give a leaf food children → rejected.
3. **Load sanity**: run planner for a full year (52 weeks of plates with varied food references) and verify shopping list / nutrition resolution stays sub-100ms per request.

### Production cutover checklist (when ready to merge)

- [ ] Backup production SQLite file.
- [ ] Dry-run migration on a copy of the prod file; verify row counts + spot-check nutrition/shopping on 5 random weeks.
- [ ] Confirm down migration restores the backup-equivalent state on the same copy.
- [ ] Merge + deploy during low-usage window.
- [ ] Post-deploy: smoke-test planner, ingredient library, shopping list.

---

## 9. Rollback

- **During development**: revert the branch; no impact on `main`.
- **After merge, before deploy**: revert merge commit; no DB changes in prod yet.
- **After deploy**: run Goose down migration. The audit tables `_migration_00013_ing_map` and `_migration_00013_comp_map` make this lossless. If for any reason the down migration fails, restore from the pre-cutover backup (step 1 of the cutover checklist).

Rollback window: keep the audit tables for at least 2 weeks post-deploy. After that, a cleanup migration drops them.

---

## 10. Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Duplicate component names confuse the comp_map generation | High | Use Go-side deterministic iteration instead of SQL window functions. Assert `SELECT COUNT(DISTINCT old_id) = COUNT(*)` on comp_map. |
| Nutrition resolver recursion stack on deep trees | Low (max depth in practice ≤ 4) | Depth limit guard (default 16) returns an error; covered by a test. |
| FTS queries used elsewhere (ingredients_fts, components_fts) | Medium | Grep for both table names across backend; update to `foods_fts` with optional `WHERE kind = X` filter at query time. |
| `UNIQUE(name) WHERE kind='leaf'` rejects migration if a component shares a name with an ingredient | Low but possible | Pre-migration check: `SELECT i.name FROM ingredients i JOIN components c ON c.name = i.name`. If non-empty, migration aborts with a clear instruction. Manual rename by user, re-run migration. |
| Frontend double maintenance during cutover | Medium | Do the refactor entirely on `feat/unified-food`. No dual-model phase in production. |
| AI importer writes to old tables | Medium | Phase 3 updates the importer. Unit tests exercise the new path. |
| Variant group behavior subtly breaks when sibling-lookup now spans kinds | Low | Keep `variant_group_id` semantically composed-only; add a DB CHECK `variant_group_id IS NULL OR kind = 'composed'`. |

---

## 11. Out of Scope (deliberate)

- Merging the *Lebensmittel* and *Rezepte* browse UIs into one unified catalog. (Future UX work; the backend is ready either way.)
- Food photos for leaf foods beyond `image_path` parity.
- Scaling recipes (×0.5, ×2) — can come later, lands on the single `reference_portions` field.
- Meal-kit / shopping planning optimizations.
- Per-household / multi-user features.

---

## 12. Acceptance Criteria (definition of done)

1. All migrations apply clean on an empty DB and on a realistic dev DB.
2. `go test -race ./...` green.
3. `golangci-lint run` clean.
4. `bun run check` green.
5. `bun run e2e --repeat-each=10 --workers=4` zero-failure.
6. Manual walk-through: user can add a banana as a snack without going through a recipe form.
7. Golden snapshot parity for nutrition + shopping list against a fixed seed.
8. Rollback dry-run proves lossless round-trip on a copy of prod data.

When all 8 items hold, merge to `main`.
