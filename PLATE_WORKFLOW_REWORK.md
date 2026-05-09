# Plate workflow rework — implementation plan

> Goal: make the plate creation/modification workflow honest about quantities, surface
> nutrition wherever it's decided, and reuse the unit/portion machinery the codebase
> already has instead of inventing new abstractions.

## Outcome at a glance

After this rework:

| Behaviour | Before | After |
|---|---|---|
| Adding a recipe (composed food) | Stepper `×0.25` to `×n.nn` | Integer stepper `1 / 2 / 3 …` portions |
| Adding an ingredient (leaf food) | Same `×1.0` stepper, secretly = "100 g" | Quantity + unit picker (`200 g`, `1 apple`, `200 ml`) with grams resolved server-side |
| Per-plate kcal/macros | Computed but not exposed | Returned with `/plates`, shown on cell, in slot sheet header, and live in tray |
| Tray layout | Picker dominates; staged additions are a footer | Plate composition + running total promoted; picker scrolls within |

The changes are partitioned into four phases; each phase is independently shippable.

---

## Architectural decision (read before any code changes)

`PlateComponent` will become **mode-aware** by *kind of food it references*, mirroring
the model that recipes already use for `FoodComponent`:

```
backend/internal/domain/plate/plate.go (current, lines 22-28)
─────────────────────────────────────────────────────────────
type PlateComponent struct {
    ID, PlateID, FoodID int64
    Portions  float64
    SortOrder int
}
```

becomes:

```go
// PlateComponent links a food onto a plate.
//
// The quantity is interpreted by food kind:
//   - Composed food (a recipe): Portions is an integer count of servings.
//                               Amount/Unit/Grams are nil.
//   - Leaf food (an ingredient): Amount + Unit are user-entered; Grams is
//                               resolved server-side via the same chain
//                               recipes use (food.Portion → units.Default →
//                               manual). Portions is nil.
type PlateComponent struct {
    ID, PlateID, FoodID int64
    Portions  *int      // composed only
    Amount    *float64  // leaf only
    Unit      *string   // leaf only — canonical unit key
    Grams     *float64  // leaf only — resolved by service, not persisted from client
    GramsSource *string // leaf only — surfaced for UI confidence badge, not persisted
    SortOrder int
}
```

**Why this shape rather than a single `Quantity` interface.** The DB column types are
different (REAL portions vs REAL amount + TEXT unit + REAL grams) and the constraint
shape is different (`portions > 0 AND portions = floor(portions)` vs `amount > 0 AND
unit != ''`). A pointer-based struct is the cheapest way to carry both modes without
adding a discriminator column — kind is already on the food row.

**Why grams is persisted.** Same reason `food_components` persists it: the resolver's
output depends on the food's portion table at the moment of save. Re-resolving on every
read would couple a plate's calorie count to subsequent edits of the food.

---

## Existing assets — reuse these, do not reinvent

| Need | What exists | Where |
|---|---|---|
| Resolve `(amount, unit)` → grams | `Service.resolveGrams` walks portion → default → manual | `backend/internal/domain/food/service.go:382-451` |
| Universal unit defaults | `units.Normalize`, `units.LookupDefault`, `units.IsCount` | `backend/internal/domain/units/defaults.go` |
| Per-food unit overrides | `food_portions` table + `food.Portion` struct | `backend/internal/domain/food/food.go:154-158`, `queries/foods.sql:138-148` |
| Per-food per-portion macros | `NutritionResolver.PerPortion(ctx, foodID)` | `backend/internal/domain/food/nutrition.go:30-61` |
| Per-plate macro aggregation | Inline in `handlers/nutrition.go:52-90` (extract!) | `backend/internal/transport/http/handlers/nutrition.go` |
| Macros struct | `nutrition.Macros` (kcal, P, F, C, fiber, sodium) | `backend/internal/domain/nutrition/calculator.go:4-11` |
| Day macro bar | `NutritionDayBar` component | `frontend/src/components/planner/NutritionDayBar.tsx` |
| Per-slot macro dots | `SlotMacroDots` component (already exists, not wired) | `frontend/src/components/planner/SlotMacroDots.tsx` |
| Daily kcal target | `useProfile().daily_kcal_target` | `frontend/src/lib/queries/profile.ts` |
| Confidence labels | `food.GramsSourceDirect / Portion / Default / Fallback / Manual` | `backend/internal/domain/food/food.go:137-143` |

**Hard rule:** if you find yourself writing a second copy of the grams chain or a
parallel macros aggregator, stop and refactor the original instead.

---

# Phase 1 — Quantity model (backend)

**Required skills (load before writing code):**
- `golang-patterns` — for the new plate-service quantity validation, the
  `food.ResolveGrams` extraction, and the migration helpers. Phase 1 is
  almost entirely Go; idiomatic error wrapping (`errors.Is`, never `==`),
  pointer-vs-value choices for the new `*int` / `*float64` fields, and
  service-construction conventions all matter here.
- `golang-testing` — required for the migration up→down→up tests, the
  table-driven plate-service cases, and the handler tests. The test
  specification below is written in this style.

## Objective

Make `PlateComponent` carry the right quantity for its food kind, resolve grams
server-side using the existing chain, and persist enough to compute macros without
re-resolving on every read.

## Files to change

### 1.1 Migration `00018_plate_component_quantity.sql` (new)

Path: `backend/db/migrations/00018_plate_component_quantity.sql`

The current column shape is set in `00013_unified_food.sql`:

```sql
CREATE TABLE plate_components (
    id         INTEGER PRIMARY KEY,
    plate_id   INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    food_id    INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    portions   REAL NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order INTEGER NOT NULL DEFAULT 0
);
```

Migration up:

1. `ALTER TABLE plate_components ADD COLUMN amount REAL`
2. `ALTER TABLE plate_components ADD COLUMN unit TEXT`
3. `ALTER TABLE plate_components ADD COLUMN grams REAL`
4. `ALTER TABLE plate_components ADD COLUMN grams_source TEXT`
5. Drop the old `portions > 0` check; replace with a CHECK that enforces XOR:
   ```
   CHECK (
     (portions IS NOT NULL AND amount IS NULL AND unit IS NULL AND grams IS NULL)
     OR
     (portions IS NULL AND amount > 0 AND unit IS NOT NULL AND grams >= 0)
   )
   ```
   On SQLite this requires the `_new` table + copy + rename pattern (used in
   `00013` and `00015`). Wrap in a transaction.
6. `UPDATE` step inside the migration: for every existing row whose food is a
   leaf, set `amount = portions * 100`, `unit = 'g'`, `grams = portions * 100`,
   `grams_source = 'direct'`, then `portions = NULL`. For composed foods, round
   `portions` to the nearest integer and store as the existing column (cast at
   read time).
7. Change `portions` from `REAL` to `INTEGER` (drop+rebuild via `_new` table).

Down: reverse — fold leaves back to `portions = grams / 100`, drop the new columns.

**Test the migration up→down→up (see `migration_00009_test.go` and
`migration_00013_test.go` for the pattern).**

### 1.2 sqlc query updates

Path: `backend/internal/adapters/sqlite/queries/planner.sql`

Update the four affected queries (`CreatePlateComponent`, `UpdatePlateComponent`,
`GetPlateComponent`, `ListPlateComponentsByPlate`) to include the new columns.
Run `sqlc generate` from the `backend` directory.

Update the `UpdatePlateComponent` query to take all five quantity columns; do not
add a separate "update grams only" query — single update keeps the consistency
contract (always re-resolve when amount or unit changes).

### 1.3 Domain model

Path: `backend/internal/domain/plate/plate.go`

Replace the `PlateComponent` struct as shown in **Architectural decision** above.
Add small helpers:

```go
func (pc PlateComponent) IsLeaf() bool { return pc.Amount != nil }
func (pc PlateComponent) IsComposed() bool { return pc.Portions != nil }
```

### 1.4 Plate service — quantity validation + grams resolution

Path: `backend/internal/domain/plate/service.go`

The plate service must take the food repo (or food service) as a constructor
dependency so it can:

1. Look up the food kind for each incoming component.
2. Reject component inputs whose quantity shape doesn't match the kind:
   - composed food + amount/unit set → `ErrInvalidInput`
   - leaf food + portions set → `ErrInvalidInput`
   - composed food + portions ≤ 0 or non-integer → `ErrInvalidInput`
3. Resolve grams for leaf components using the *same chain* recipes use.

**Reuse, don't duplicate.** The cleanest move is to lift `Service.resolveGrams`
out of `food.Service` (currently `service.go:382-451`) into a small package-level
function or a `food.GramsResolver` type that both services call. Suggested
shape:

```go
// backend/internal/domain/food/grams.go (new)
package food

// ResolveGrams resolves Amount + Unit to Grams for a leaf food using:
//   1. Food-specific portion override
//   2. Universal mass/volume default
//   3. Manual grams (only when caller passed an explicit grams value)
//
// Returns the resolved grams and the source label (one of GramsSource*).
// foodID must reference a leaf food; the function assumes the caller has
// validated that.
func ResolveGrams(
    ctx context.Context,
    portions PortionLookup,
    foodID int64,
    amount float64,
    unit string,
    manualGrams float64,
) (grams float64, source string, err error)

type PortionLookup interface {
    ListPortions(ctx context.Context, foodID int64) ([]Portion, error)
}
```

Then both `Service.resolveGrams` and the new plate service call this. The
existing `Service.resolveGrams` becomes a thin wrapper that loops over
`children`. No behavioural change for recipes; new behaviour for plates.

### 1.5 Plate handler / DTOs

Paths:
- `backend/internal/transport/http/handlers/plate.go` (or wherever Plate lives)
- `backend/internal/transport/http/handlers/plate_response.go`

Update request/response types:

```go
type addPlateComponentRequest struct {
    FoodID   int64    `json:"food_id"`
    Portions *int     `json:"portions,omitempty"` // composed only
    Amount   *float64 `json:"amount,omitempty"`   // leaf only
    Unit     *string  `json:"unit,omitempty"`     // leaf only
}

type plateComponentResponse struct {
    ID          int64    `json:"id"`
    FoodID      int64    `json:"food_id"`
    Portions    *int     `json:"portions,omitempty"`
    Amount      *float64 `json:"amount,omitempty"`
    Unit        *string  `json:"unit,omitempty"`
    Grams       *float64 `json:"grams,omitempty"`
    GramsSource *string  `json:"grams_source,omitempty"`
    SortOrder   int      `json:"sort_order"`
}
```

The validator must require exactly one of (`portions`) or (`amount` + `unit`).

### 1.6 Nutrition calculator

Path: `backend/internal/domain/nutrition/calculator.go`

`PlateTotal` currently multiplies macros by `Portions`. With the new model, the
calculator's input shape is ambiguous. Choose **one** of:

- **(Recommended) Resolve to a single multiplier upstream.** Keep
  `PlateComponentInput { Macros, Portions }` exactly as is. Upstream code (the
  nutrition handler) computes the multiplier per component:
    - composed: `multiplier = portions` (macros from `NutritionResolver.PerPortion`)
    - leaf:     `multiplier = grams / 100` (macros are per-100g from the leaf
                food row)
  This keeps `nutrition.PlateTotal` and `nutrition.WeekTotals` untouched — they
  already do the right thing once you give them a normalised pair.

The handler currently does this only partially (`handlers/nutrition.go:52-90`)
and calls macros "per-portion". Rename internal vars to `multiplier` and split
the lookup into "per-portion for composed" + "per-100g for leaves" branches.

### 1.7 Per-plate macros endpoint

The day-totals endpoint already aggregates per-plate macros internally; expose
them.

**Option A (recommended)** — extend the existing range endpoint:
extract the per-plate macro computation in `handlers/nutrition.go:52-90` into a
helper, and reuse it from a new `GET /api/plates/macros?from=&to=` handler that
returns:

```json
{
  "plates": [
    { "plate_id": 7, "macros": { "kcal": 612, "protein": 38.4, ... } },
    ...
  ]
}
```

Wired in `router.go` next to the existing `api.Get("/nutrition", ...)`. Keep
both endpoints — the per-day endpoint stays useful for the day-headers and the
shopping export.

**Option B** — embed `macros` inside each component on the existing `GET /plates`
response. Don't pick this; it conflates resource shape with derived data and
double-pays the nutrition resolve cost when the planner refetches plates after
trivial mutations (note edits, skip toggles).

## Phase 1 — test specification

### 1.1 Migration tests
*Path: `backend/internal/adapters/sqlite/migration_00018_test.go`*

Pattern: copy `migration_00013_test.go`. Cases:

- `up_creates_columns`: after up, querying `PRAGMA table_info(plate_components)`
  returns `amount, unit, grams, grams_source` and `portions INTEGER`.
- `up_backfills_leaf_components_to_grams`: seed a row pre-migration with
  `food_id=<leaf>`, `portions=1.5`. After up, expect `amount=150, unit='g',
  grams=150, grams_source='direct', portions=NULL`.
- `up_backfills_composed_components_to_int_portions`: seed `portions=2.5` on
  a composed food. After up, expect `portions=3` (round half up; document the
  rule). Manual review note in the migration.
- `down_reverses_to_real_portions_column`: run down; row reads
  `portions=2.0` (or whatever maps back); new columns gone.
- `up_down_up_idempotent`: schema after second up matches first.

### 1.2 Plate service tests
*Path: `backend/internal/domain/plate/service_test.go` — extend existing file.*

Use table-driven tests (per `golang-testing` skill). Ground truth via
`testhelper.NewTestDB()` — never mock the DB.

Cases:

- `AddComponent_Composed_Portions_OK`: add `{food_id: <bolognese>, portions: 2}`
  → component reads back with `portions=2, amount=nil, unit=nil`.
- `AddComponent_Composed_Rejects_Amount`: `{food_id: <bolognese>, amount: 100,
  unit: "g"}` → `ErrInvalidInput` with key
  `error.plate.invalid_quantity_for_composed`.
- `AddComponent_Composed_Rejects_Fractional_Portions`: `{portions: 1.5}` (sent
  via float in JSON, integer-required) → `ErrInvalidInput`.
- `AddComponent_Composed_Rejects_Zero_Portions`: → `ErrInvalidInput`.
- `AddComponent_Leaf_Mass_Direct`: `{food_id: <basmatireis>, amount: 200,
  unit: "g"}` → `grams=200, grams_source='direct'`.
- `AddComponent_Leaf_Mass_Default`: `{amount: 7, unit: "oz"}` → `grams≈198.4,
  grams_source='default'`.
- `AddComponent_Leaf_Volume_Fallback`: `{amount: 200, unit: "ml"}` →
  `grams=200, grams_source='fallback'`.
- `AddComponent_Leaf_Portion_Override`: seed apple with portion `1 apple = 180g`.
  `{amount: 1, unit: "apple"}` → `grams=180, grams_source='portion'`.
- `AddComponent_Leaf_Count_Without_Portion_Errors`: `{amount: 1, unit: "slice"}`
  on a food without that portion → `ErrInvalidInput` carrying message key
  `error.plate.unit_requires_portion`.
- `AddComponent_Leaf_Rejects_Portions`: leaf food + `{portions: 1}` →
  `ErrInvalidInput`.
- `AddComponent_Leaf_Rejects_Both`: `{portions: 1, amount: 100, unit: "g"}` →
  `ErrInvalidInput`.
- `UpdateComponentQuantity_Reresolves_Grams`: change unit from `g` to `apple`;
  `grams_source` flips from `direct` to `portion` and grams updates.
- `RemoveComponent_LastOne_Cascades_To_PlateDelete`: existing behaviour, regress.

### 1.3 Grams resolver shared-function tests
*Path: `backend/internal/domain/food/grams_test.go` (new)*

If the resolver is extracted, mirror cases from existing `service_test.go`
covering `resolveGrams` and add: `unknown_unit_errors`, `manual_grams_with_count_unit_succeeds`.

### 1.4 Handler tests
*Path: `backend/internal/transport/http/handlers/plate_test.go`*

Cases (HTTP-level):

- `POST /plates/:id/components` with composed quantity returns `201` and the
  echo includes `portions`, omits `amount/unit/grams`.
- Same with leaf returns `201` and echo includes `amount, unit, grams, grams_source`.
- Malformed: both quantity shapes → `400` with `error.plate.invalid_quantity_shape`.
- Malformed: `unit` not normalisable to a known one and food lacks portion →
  `400` with `error.plate.unit_requires_portion`.
- `GET /plates?from=&to=` round-trips both shapes verbatim.

### 1.5 New macros endpoint test
*Path: `backend/internal/transport/http/handlers/plate_macros_test.go` (new)*

- Empty range → `{ "plates": [] }`.
- Plate with one composed component (Bolognese, 1 portion) → kcal matches the
  `NutritionResolver.PerPortion` value.
- Plate with one leaf component (200 g rice) → kcal matches `(per100g.kcal *
  2.0)`.
- Plate with both → kcal is the sum.
- Skipped plate → still returned (UI uses macros to dim cells, not to hide
  them); add a `skipped: bool` field if the UI needs it.

### Phase 1 acceptance

- All Phase 1 tests pass with `-race`.
- `sqlc generate` produces no diff after the queries change.
- Existing planner e2e (`frontend/e2e/`) still passes — the API DTOs are a
  superset; unchanged clients should keep working until the frontend lands.

---

# Phase 2 — Surface macros everywhere they're decided

**Required skills (load before writing code):**
- `vercel-react-best-practices` — the new `usePlateMacros` and
  `useFoodMacros` hooks must batch correctly, share a query key with the
  plates list (so invalidations cascade), and use `useDeferredValue` for
  the running total to avoid per-keystroke fetches. The skill covers
  TanStack Query patterns, render-time work, and the
  `react-hooks/set-state-in-effect` lint trap noted in CLAUDE.md.
- `vitest` — for the new `SlotCell.test.tsx`, `SlotSheet.test.tsx`, and
  the `usePlateMacros` query test. Reuse `renderWithRouter` from
  `@/test/render` and shared fixtures from `@/test/fixtures`; mock at the
  `@/lib/api/*` module level so TanStack Query keeps working normally.
  (`playwright-best-practices` also applies to the §2.3 e2e suite —
  load it when starting that file specifically.)

This phase adds **read-only display** of macros. It must be shippable on top of
Phase 1 without touching the picker UI.

## Files to change

### 2.1 API client + query hook

Path: `frontend/src/lib/api/plates.ts`

Extend `PlateComponent` and `Plate`:

```ts
export interface PlateComponent {
  id: number
  plate_id: number
  food_id: number
  portions?: number      // composed only
  amount?: number        // leaf only
  unit?: string          // leaf only
  grams?: number         // leaf only — server-resolved
  grams_source?: string  // leaf only
  sort_order: number
}
```

Add:

```ts
export interface PlateMacros {
  plate_id: number
  macros: MacrosResponse
}

export function listPlateMacros(from: string, to: string): Promise<{ plates: PlateMacros[] }>
```

Path: `frontend/src/lib/queries/plates.ts` — add `usePlateMacros(from, to)`. The
query key should mirror the plates list key (same `from`/`to` pair). Invalidate
the macros query on the same mutations that already invalidate the plates list
(add/remove/update component, swap, move, skip).

### 2.2 Slot cell — kcal pill + macro dots

Path: `frontend/src/components/planner/SlotCell.tsx`

The 80 px dead band is intentional (see comment at lines 65-71). Reuse:

- `SlotMacroDots` (`./SlotMacroDots.tsx`) for P/F/C dots
- A new `kcal` line in the existing layout, e.g. `620 kcal` rendered in
  `font-mono tabular-nums` to align across cells

Acceptance: with the macros query loaded, every planted cell displays kcal at
the bottom and a small dot row showing macro distribution. While the macros
query is loading, render a `tabular-nums` placeholder so the layout doesn't
shift.

### 2.3 Slot sheet — header summary block + per-component contribution

Path: `frontend/src/components/planner/SlotSheet.tsx`

Replace the description line `"1 component planned"` with a compact macro block
under the title:

- Big number: kcal
- Row: P, F, C in grams (and bar showing the kcal proportions)

For each `ComponentRow`, render the component's individual kcal contribution
next to the stepper. Use the same `usePlateMacros` query and look up by plate
id. Keep the existing `PortionStepper` for now — Phase 3 replaces it.

The existing `slot_sheet.description_one/other` i18n key stays; consider adding
a new key `slot_sheet.macros_summary` for the macro line.

### 2.4 Tray — running total

Path: `frontend/src/components/planner/ComponentTraySheet.tsx`

Add a sticky summary inside `TrayFooter` (above the `tray-staged-list`):

- Total kcal of staged items (sum of per-portion macros × staged portions).
  Until Phase 3, leaf items still use `×portions` semantics, so during the
  transition window keep the same macros approach the slot sheet uses.

For the running total, **fetch per-portion macros on demand** using a new
`useFoodMacros(foodIds: number[])` hook. Don't refetch on every keystroke —
debounce with `useDeferredValue` (already used elsewhere in the file).

Backend support: add `GET /api/foods/macros?ids=1,2,3` returning
`{ foods: [{ food_id, macros }] }` where `macros` is per-portion (composed) or
per-100g (leaf). The endpoint maps directly to `NutritionResolver.PerPortion`
which already encodes that distinction.

## Phase 2 — test specification

### 2.1 API contract tests

Path: `backend/internal/transport/http/handlers/foods_macros_test.go` (new)

- `GET /foods/macros?ids=1,2,3` returns macros for each id in the requested
  order.
- Unknown id → element omitted (don't 404 the whole batch); document this in
  the handler comment.
- Empty `ids` query → `400`.

### 2.2 Frontend unit tests (vitest)

Path: `frontend/src/components/planner/SlotCell.test.tsx`

Add:

- `renders_kcal_pill_for_planted_cell`: with macros prop set, finds
  `data-testid="slot-cell-kcal"` containing the formatted number.
- `renders_skeleton_while_macros_loading`: macros undefined → renders the
  placeholder, no layout shift in computed height.

Path: `frontend/src/components/planner/SlotSheet.test.tsx` (new — pattern from
`SlotSheet`'s sibling tests)

- `renders_macro_summary_in_header`
- `renders_per_component_kcal_contribution`

Path: `frontend/src/components/planner/ComponentTraySheet.test.tsx` (extend)

- `running_total_updates_when_staging_a_food`: stage Bolognese (1 portion);
  expect `data-testid="tray-running-kcal"` text matches that food's per-portion
  kcal.
- `running_total_handles_leaf_items_with_grams`: stage 200 g rice; total =
  per-100g × 2.

Path: `frontend/src/lib/queries/plates.test.tsx` (extend)

- `usePlateMacros_invalidates_on_addComponent`: mock the API; mutate; assert
  refetch was triggered.

### 2.3 e2e (Playwright)

Path: `frontend/e2e/plate-macros.spec.ts` (new)

Seed via direct API (`http://localhost:8080`), unique names with `crypto.randomUUID`:

- Plant a single-component plate. Cell shows kcal that matches the value
  returned by `GET /api/plates/macros`.
- Open the slot sheet. Header shows the same kcal.
- Open the tray and stage one food. Running total kcal matches the per-portion
  macros from `GET /api/foods/macros`.
- Use `waitForResponse` on `/api/plates/macros` after each mutation; never
  `waitForTimeout`.

### Phase 2 acceptance

- Cells, slot sheet, and tray all show macros that agree with the canonical
  `NutritionResolver.PerPortion` numbers.
- No visible flicker on planner refresh — macro values remain stable across
  trivial mutations (note edit, feedback toggle).

---

# Phase 3 — Kind-aware quantity input

**Required skills (load before writing code):**
- `vercel-composition-patterns` — the central design problem here is
  **two distinct quantity controls behind a single call site**. The skill's
  guidance on compound components, kind-discriminated APIs, and avoiding
  boolean-prop proliferation is exactly what `PortionStepper` /
  `QuantityUnitInput` need. The wrong shape (one component with a `mode`
  prop) is a tempting trap; the skill helps you avoid it.
- `frontend-design` — `QuantityUnitInput` is a new editable surface that
  must match the Botanical Atelier system (per the user-memory note on
  the design rollout). Quick-amount chips, the confidence pill, and the
  inline grams preview all need to read as one cohesive control, not
  three stacked widgets.

The picker stops lying to the user. Composed foods get an integer stepper; leaf
foods get a quantity + unit picker.

## Files to change

### 3.1 Quantity input components (new, shared)

Path: `frontend/src/components/component/PortionStepper.tsx` (new)

Pure integer stepper. Min 1, max ~20, +/− buttons + tabular-nums display. No
"0.25" anywhere in this component — it replaces (not extends) the half-baked
ones currently inlined in `ComponentTraySheet.tsx:783-827` and
`SlotSheet.tsx:562-605`.

Path: `frontend/src/components/component/QuantityUnitInput.tsx` (new)

For leaf foods. Takes a `Food` and returns `{ amount, unit }` to its parent.
Internal layout:

- Numeric input (mobile-friendly, `inputMode="decimal"`)
- Unit selector (a `<Select>` of the food's `Portions` table entries followed
  by the universal masses (g/kg/oz/lb) and volumes (ml/l/cup/tbsp/tsp))
- Live preview "≈ 180 g" with a confidence pill (`portion`, `default`,
  `fallback`) using the same colour vocabulary the food editor uses for its
  ingredient lines

Default unit selection rule (document in the component header):

1. Most recently used unit on this food in any plate (last 30 days) — read from
   a small `food_recent_units` query, see 3.2 below.
2. The first entry in the food's `Portions` table.
3. `g` if the food has any portions, otherwise `g` (mass default).

Add quick-amount chips beneath the input keyed off the unit:

- For `g`: `50 / 100 / 150 / 200 / 300`
- For piece-like units (anything in the food's Portion table): `1 / 2 / 3`
- For volume: `100 / 200 / 250 / 500`

### 3.2 Recent unit query (small backend helper)

Path: `backend/internal/adapters/sqlite/queries/planner.sql`

Add:

```sql
-- name: RecentUnitForFood :one
SELECT unit FROM plate_components
WHERE food_id = ? AND unit IS NOT NULL
ORDER BY id DESC
LIMIT 1;
```

Wire through plate service + a `GET /api/foods/:id/recent-unit` endpoint, or
piggyback on the food detail GET. Don't add a frontend localStorage cache —
this is a read-from-truth situation.

### 3.3 Wire the new inputs into the tray

Path: `frontend/src/components/planner/ComponentTraySheet.tsx`

`StagedRow` (line 733) and the tray reducer (line 903) need to track the new
quantity model:

```ts
export interface TrayItem {
  food: Food
  quantity:
    | { kind: "composed"; portions: number }
    | { kind: "leaf"; amount: number; unit: string }
}
```

When staging, branch on `food.kind`:

- composed → `quantity: { kind: "composed", portions: 1 }`
- leaf → `quantity: { kind: "leaf", amount: <default by unit>, unit: <default> }`

`onCommit` sends the new shape per food kind:

```ts
{ food_id, portions: 2 }
{ food_id, amount: 200, unit: "g" }
```

Replace the inlined `PortionStepper` with the two new components, picked by
`food.kind`.

### 3.4 Wire the new inputs into the slot sheet

Path: `frontend/src/components/planner/SlotSheet.tsx`

`ComponentRow` (line 452) currently uses an inline `PortionStepper`. Replace
with a kind-aware control:

- composed → `PortionStepper`
- leaf → `QuantityUnitInput` (compact mode — single line, no chips)

The mutation hook `useUpdatePlateComponentPortions` becomes
`useUpdatePlateComponentQuantity` and accepts the unified shape. Keep the
optimistic-update logic; only the payload shape changes.

Path: `frontend/src/lib/queries/plates.ts` and
`frontend/src/lib/queries/plate-patches.ts` — update the patch helpers' shape;
mirror the change in `plate-patches.test.ts`.

### 3.5 i18n

Path: `frontend/src/lib/i18n/en.json` and `de.json`

Add:

- `plate.quantity.portions_label` → "Portions"
- `plate.quantity.amount_label` → "Amount"
- `plate.quantity.unit_label` → "Unit"
- `plate.quantity.confidence.portion` → "exact"
- `plate.quantity.confidence.default` → "approx"
- `plate.quantity.confidence.fallback` → "estimate"
- `plate.quantity.unknown_unit` → "Unknown unit"

## Phase 3 — test specification

### 3.1 Unit tests (vitest)

Path: `frontend/src/components/component/PortionStepper.test.tsx` (new)

- Renders value as integer.
- `+` increments by 1, `−` decrements by 1.
- Cannot go below 1.
- Aria-valuenow updates.

Path: `frontend/src/components/component/QuantityUnitInput.test.tsx` (new)

- Default unit = first entry from food's `portions`.
- Default unit falls back to `g` when food has no portions.
- Quick chips render appropriate to selected unit.
- Confidence pill shows `portion` when food's portion table contains the
  selected unit; `default` for `oz`; `fallback` for `ml` on a non-volume food.
- Emits `{ amount, unit }` on change; never emits `amount = 0`.

### 3.2 Tray reducer tests (vitest)

Path: `frontend/src/components/planner/ComponentTraySheet.test.tsx` (extend)

Existing `trayReducer` tests use the old portions shape — rewrite them around
the new union:

- `stage_composed_initialises_portions_1`
- `stage_leaf_initialises_with_default_unit_and_amount`
- `stage_existing_composed_increments_portions_by_1` (was 0.25)
- `stage_existing_leaf_increments_amount_by_1_unit_when_count`
- `stage_existing_leaf_increments_amount_by_50_when_grams`
- `commit_payload_for_composed_uses_portions_only`
- `commit_payload_for_leaf_uses_amount_unit`

### 3.3 Slot sheet tests

Path: `frontend/src/components/planner/SlotSheet.test.tsx`

- `composed_component_uses_integer_stepper`
- `leaf_component_uses_quantity_unit_input`
- `changing_unit_updates_grams_via_mutation` (mock the API; assert the
  payload shape on the wire)

### 3.4 Backend recent-unit endpoint

Path: `backend/internal/transport/http/handlers/food_recent_unit_test.go` (new)

- No prior plates with that food → returns `{ unit: null }`.
- Most recent component used `apple` → returns `{ unit: "apple" }`.
- Most recent was a composed plate (no unit set) → falls through to the next
  most recent leaf usage.

### 3.5 e2e

Path: `frontend/e2e/plate-quantity.spec.ts` (new)

- Plant a recipe (composed) — the picker shows an integer stepper. `+` goes
  `1 → 2 → 3`. `−` from `1` is disabled (or no-op).
- Plant a leaf (rice) — the picker shows quantity + unit. Default unit is `g`,
  default amount is `100`. Quick chip `200` sets amount to `200`. Save. The
  cell's kcal matches `(rice.kcal100g * 2)`.
- Plant a leaf with portion override (apple `1 apple = 180g`) — default unit
  is `apple`. Switch to `g`; the live grams preview disappears (you're now in
  direct mode).
- After save, the slot sheet shows the leaf as `200 g` and the composed as
  `× 2`. Editing the leaf to `300 g` updates the cell kcal.

Stability bar: `--repeat-each=10 --workers=4` zero failures.

### Phase 3 acceptance

- No `0.25` portion increment exists anywhere in the planner UI.
- For every leaf component on a plate, the user can read its grams in the
  slot sheet and change the unit.
- The picker prevents nonsense input (cannot send a leaf with portions, cannot
  send a composed with amount/unit) — server-side validation is the safety
  net, not the only line of defence.

---

# Phase 4 — Composition-first tray layout

**Required skills (load before writing code):**
- `frontend-design` — Phase 4 is layout-led. The three-region restructure
  of `ComponentTraySheet`, the `DraftPlatePreview` hierarchy, the muted
  styling for existing-vs-staged components, and the mobile collapse
  behaviour are all design decisions that need the skill's guidance to
  land coherently within Botanical Atelier.
- `frontend-review` — once the layout lands, run this skill against the
  tray sheet (treat it as a "page" — it's a full-screen sheet on mobile
  and a 480px right rail on desktop). The skill's structured 1–5 ratings
  and prioritised fix list are how you confirm the rework actually
  resolves the original "added meals are at the bottom almost out of
  scope" complaint instead of just shuffling boxes around.

The picker UI is restructured so the **plate composition + running total** is
the visual anchor, with the food browser below.

This phase is layout-only — no new data, no new endpoints. Ship it after
Phase 3 has stabilised.

## Files to change

Path: `frontend/src/components/planner/ComponentTraySheet.tsx`

Restructure `ComponentTraySheetBody` (line 154) into three vertical regions
inside the sheet:

```
┌──────────────────────────────────────┐
│ Eyebrow + Title (existing)           │
├──────────────────────────────────────┤
│ DRAFT PLATE PREVIEW                  │  ← new top region
│ - Hero image (first staged item)     │
│ - Component pills (name + qty)       │
│ - Running total (kcal + macro bar)   │
├──────────────────────────────────────┤
│ Search                               │  ← compresses on scroll
│ Tabs                                 │
│ Role chips                           │
│ Results list (scrolls within region) │
├──────────────────────────────────────┤
│ Cancel / Add N (existing footer)     │
└──────────────────────────────────────┘
```

Reuse:
- `SlotHero` for the preview hero image
- `SlotMacroDots` for the macro distribution
- `NutritionDayBar`'s palette for the macro bar

The "draft plate preview" is a **new presentational component**:
`frontend/src/components/planner/DraftPlatePreview.tsx`. Takes
`{ items: TrayItem[], runningTotal: Macros, dayKcalTarget: number | null }`.

When the tray opens on a slot that already has a plate, the preview shows the
**existing plate components first** (greyed slightly), then staged additions
on top of them with a `+` indicator. This solves the "what am I editing?"
problem the user mentioned.

Mobile layout (`side="bottom"`): the preview collapses to a single-line
summary unless tapped, to preserve vertical real estate for the search results.

## Phase 4 — test specification

### 4.1 Visual regression

Path: `frontend/e2e/plate-tray-layout.spec.ts` (new)

- Open empty slot → preview region renders empty state with "Nothing yet"
  copy.
- Stage one item → preview shows hero + chip + non-zero running total.
- Stage three items → preview shows three chips and the running total reflects
  the sum.
- Open the tray on a non-empty plate → preview shows existing components in
  muted style above the staged ones.
- Mobile viewport (`devices['iPhone 13']`) — preview is single-line collapsed;
  expanding it shows the full list.
- Screenshot snapshots locked at `--update-snapshots` for review.

### 4.2 Vitest

Path: `frontend/src/components/planner/DraftPlatePreview.test.tsx` (new)

- Renders running total kcal correctly.
- Renders existing plate components when `existing` prop is set.
- Empty state when no items and no existing plate.

### Phase 4 acceptance

- The picker's primary visual focus is the plate being built, not the food
  library.
- The user mentioned in the original report ("added meals are at the bottom
  almost out of scope") is no longer reproducible.

---

# Cross-cutting concerns

## What does **not** change

- The grams resolver chain (Phase 1 reuses it; the chain itself is correct).
- `nutrition.PlateTotal` and `nutrition.WeekTotals` (the unit they multiply by
  is renamed conceptually but the math is unchanged).
- Templates. A template currently stores `food_id + portions`. Phase 1 must
  either:
  - keep the template shape as-is and translate at apply time (compose:
    portions, leaf: `portions × 100 g`), with a note in the migration that
    templates are intentionally low-fidelity; or
  - extend the template shape with the same quantity model.

  **Recommendation:** punt to a follow-up. Most templates the user has today
  are recipe-shaped, so the loss of fidelity for ingredient-typed templates is
  acceptable. Add a TODO in the template handler.
- Daily/weekly nutrition aggregation surfaces (`NutritionDayBar`,
  `NutritionWeekSummary`). They consume the existing per-day endpoint
  unchanged.

## i18n

Every new user-facing string must land in both `en.json` and `de.json` in
`frontend/src/lib/i18n/`. The existing `plate.*`, `tray.*`, `slot_sheet.*`,
`component.*` namespaces already exist — extend them, don't invent new ones.

## Permissions / auth

None — single-user LAN deployment.

## Telemetry

None added. No analytics in this codebase.

## Performance budget

- `GET /plates/macros?from=&to=` for a 7-day window must return < 50 ms on
  current dev DB sizes (~80 plates per week, ~5 components each). The
  resolver caches per food_id within the request — re-confirm with the
  existing `nutrition.go:53` pattern.
- Tray running total mustn't refetch per keystroke. Use `useDeferredValue` on
  staged-tray state changes, batch the macros lookup with a single
  `/foods/macros?ids=...` request when the staged set changes.

## Backwards compatibility

There are no other clients (mobile app, third-party API). Phase 1's API change
is a breaking change *for the bundled SPA only*. Land Phase 1's backend +
Phase 1's frontend API client (the type changes only) in the same release — no
gradual rollout required.

---

# Test stability bar

| Suite | Bar |
|---|---|
| Go unit + integration | `go test -race ./...` clean, every phase. |
| Migration up→down→up | Schema-equivalent; assert via `PRAGMA table_info`. |
| Vitest | `bun run test` clean every phase. |
| Playwright | `bun run e2e --repeat-each=10 --workers=4` zero failures by end of each phase. |

`bun run check` (lint + typecheck + unit tests) must pass before each PR.

---

# Suggested PR slicing

| PR | Phase | Touches |
|---|---|---|
| 1 | 1 — backend only | migration 00018, queries, plate domain, plate handler, nutrition handler refactor, new `/plates/macros` endpoint |
| 2 | 1 — frontend types only | `lib/api/plates.ts` shape; existing UI keeps working since old leaf rows backfill to `200g` etc. |
| 3 | 2 — display | `SlotCell` kcal + dots, `SlotSheet` header summary, tray running total, `/foods/macros` endpoint |
| 4 | 3 — input | new `PortionStepper` + `QuantityUnitInput`, swap into tray + slot sheet, recent-unit endpoint |
| 5 | 4 — layout | `DraftPlatePreview`, restructure `ComponentTraySheet` |

PRs 3, 4, 5 each pick up the previous phase's contract; landing them in order
lets each commit the user manually verifies in the running app give a clear
single-axis improvement.

---

# Verification walkthrough (run after each PR)

The user explicitly cares about the live behaviour. Use `agent-browser` against
the docker compose stack (port 8080):

1. Plant a recipe (composed). Check stepper increments by 1.
2. Plant a leaf ingredient (e.g. 200 g rice). Check the unit dropdown defaults
   correctly, the live grams preview shows, and the cell renders the right
   kcal.
3. Open the slot sheet. Verify the macro summary, per-component contributions,
   and the new quantity inputs.
4. Open the tray on a non-empty plate. Verify the draft preview shows existing
   components.
5. Stage two items, watch the running total update without a network round
   trip per keystroke.
6. Cross-check kcal against `curl http://localhost:8080/api/plates/macros?from=YYYY-MM-DD&to=YYYY-MM-DD`.

If step 6 disagrees with the UI, the bug is in the frontend display layer, not
the resolver.
