# Plantry — Software Architecture

This document is the technical contract for the Plantry redesign. It defines the stack, the layering, the data model, the API surface, the AI integration shape, the frontend architecture, and the cross-cutting concerns. Read this before writing any code.

---

## 1. Goals and Non-Goals

### Architectural Goals

1. **Testable in pure units.** Every business rule must be exercisable with a Go test that touches no database, no HTTP, no LLM. This forces a clean domain layer.
2. **Replaceable infrastructure.** SQLite, OpenAI, Anthropic, filesystem image storage, OFF, and FDC must all be hidden behind ports. Swapping any one of them must not touch the domain.
3. **Single binary, no CGO.** The production artefact is one pure-Go binary with the SvelteKit static build embedded. Must cross-compile to ARMv7 and ARM64 for Raspberry Pi.
4. **Deterministic migrations.** Schema changes go through goose, run on startup, and never touch user data in handlers.
5. **One source of truth for nutrition math.** Exactly one implementation. Used by handlers, AI tools, shopping list, and frontend (via API, not reimplementation).
6. **Vertical tests.** Every feature has a unit test in the domain, an adapter test at the boundary, and an e2e test through the browser.

### Non-Goals

- Horizontal scaling (single-process by design).
- Multi-tenancy (single household per deployment).
- Schema-level extensibility (new fields go through migrations, not generic JSON blobs — with two named exceptions).
- Offline sync / conflict resolution (the client talks to a single backend).

---

## 2. Technology Stack

### Backend

| Concern           | Choice                                           | Rationale                                              |
| ----------------- | ------------------------------------------------ | ------------------------------------------------------ |
| Language          | Go 1.25                                          | Pure-Go ecosystem, cross-compile, tiny binary          |
| Router            | chi v5                                           | Minimal, middleware-friendly, well-tested              |
| Database          | SQLite via `modernc.org/sqlite`                  | Pure Go, no CGO, single file, WAL, FTS5                |
| Migrations        | `pressly/goose` v3                               | Pure Go, embeddable, versioned                         |
| Query layer       | `sqlc`                                           | Compile-time type-safe SQL → Go, zero runtime cost     |
| Dynamic queries   | `Masterminds/squirrel` (sparingly)               | For the 3–4 endpoints with optional filters            |
| Assertions        | `stretchr/testify`                               | Project convention, clear failure messages             |
| LLM (OpenAI)      | stdlib `net/http`                                | No SDK lock-in, minimal dependency surface             |
| LLM (Anthropic)   | stdlib `net/http`                                | Same                                                   |
| SSE               | stdlib `net/http` with flusher                   | Native, zero deps                                      |
| Image handling    | `disintegration/imaging` + stdlib `image/jpeg`   | Pure Go resize/crop/encode with quality control        |
| Config            | env vars via stdlib                              | Twelve-factor, no config file                          |
| Logging           | `log/slog`                                       | Stdlib, structured, zero deps                          |

**Hard constraints:** no CGO, no C-dependent libraries, no native extensions. Every dependency must cross-compile to `GOARCH=arm GOARM=7` and `GOARCH=arm64` from macOS/Linux.

### Frontend

| Concern          | Choice                                   | Rationale                                              |
| ---------------- | ---------------------------------------- | ------------------------------------------------------ |
| Bundler          | Vite 6                                   | SPA-first, fast dev, tiny prod output                  |
| UI runtime       | React 19 + TypeScript                    | Battletested, vast ecosystem, long-term stable         |
| Styling          | Tailwind v4                              | OKLch first-class, zero runtime cost                   |
| Component library| shadcn/ui                                | Gold-standard copy-paste primitives, a11y by default   |
| Icons            | Lucide React                             | Tree-shakeable, consistent                             |
| Routing          | TanStack Router                          | Fully type-safe params, file-based, SPA-native         |
| Server state     | TanStack Query                           | Caching, mutations, optimistic updates with rollback   |
| Client state     | Zustand                                  | ~1 KB, only for ephemeral UI state                     |
| Forms            | `react-hook-form` + `zod`                | Uncontrolled, fast, schema-validated                   |
| Drag-and-drop    | `dnd-kit`                                | Accessible, composable, the React standard            |
| Barcode scanning | `BarcodeDetector` API + `@zxing/library` | Native where supported, cross-browser fallback         |
| HTTP             | `fetch` wrapped in typed client          | Consumed through TanStack Query hooks                  |
| i18n             | `react-i18next`                          | Mature, lazy load, locale switching                    |
| Unit tests       | Vitest + React Testing Library           | Fast, colocated with Vite                              |
| e2e tests        | Playwright                               | Mature, trace viewer, multi-browser                    |
| Package manager  | Bun                                      | Fast installs, lockfile convention                     |

### Colors (kept from current design)

OKLch-based botanical theme, sage green primary. The following tokens are authoritative:

```
--primary:    oklch(0.55 0.08 145)   /* sage green ~ #4a654d */
--secondary:  oklch(0.92 0.03 145)
--accent:     oklch(0.88 0.06 75)    /* warm amber */
--background: oklch(0.98 0.01 85)    /* warm off-white */
--foreground: oklch(0.22 0.02 145)   /* deep moss */
--muted:      oklch(0.94 0.02 85)
--border:     oklch(0.88 0.02 110)
--destructive: oklch(0.55 0.18 25)
--chart-protein: oklch(0.62 0.12 25)
--chart-carbs:   oklch(0.72 0.10 85)
--chart-fat:     oklch(0.66 0.11 55)
--chart-fiber:   oklch(0.55 0.08 145)
```

Dark mode mirrors with inverted lightness. Full token list lives in `frontend/src/routes/layout.css`.

### Deployment

Single Docker image (multi-stage build):

1. **Stage A (Bun):** installs frontend deps, builds static SPA.
2. **Stage B (Go):** copies Stage A output into `static/`, compiles backend with `//go:embed` of `static/`, outputs single binary.
3. **Stage C (Alpine):** copies binary only. Final image ~30 MB. Volume `/data` holds the SQLite file and `images/` directory.

Environment variables:

```
PLANTRY_PORT=8080
PLANTRY_DB_PATH=/data/plantry.db
PLANTRY_FDC_API_KEY=            # optional
PLANTRY_AI_PROVIDER=            # openai | anthropic | (empty)
PLANTRY_AI_MODEL=
PLANTRY_AI_API_KEY=
PLANTRY_AUTH_PASSWORD=          # optional single password gate
PLANTRY_LOG_LEVEL=info
```

---

## 3. Layering (Hexagonal-Lite)

Three layers, strict inward dependency: **transport → domain ← adapters**. The domain depends on nothing but stdlib. Adapters depend on the domain (they implement its ports). Transport depends on both (it wires them up).

### Backend directory layout

```
backend/
├── cmd/plantry/main.go               # wiring: env, db, adapters, router, server
├── internal/
│   ├── domain/                       # PURE. No imports from adapters or transport.
│   │   ├── ingredient/               # aggregate, value objects, repo port, service
│   │   ├── component/                # aggregate (formerly "recipe"), role, variant
│   │   ├── plate/                    # plate + plate_component, role-aware swap
│   │   ├── planner/                  # week, plate composition, copy-week policy
│   │   ├── nutrition/                # PURE calculator (inputs → macros)
│   │   ├── shopping/                 # PURE aggregator (plates → list)
│   │   ├── feedback/                 # feedback events + profile update rules
│   │   ├── profile/                  # user profile aggregate + targets
│   │   ├── template/                 # saved plate compositions
│   │   ├── llm/                      # LLM port interface + tool schema types
│   │   ├── importer/                 # URL → draft component port + orchestration
│   │   └── errors.go                 # domain sentinel errors
│   ├── adapters/
│   │   ├── sqlite/                   # sqlc-generated queries + repo adapters + migrations runner
│   │   ├── openai/                   # implements domain/llm.Client
│   │   ├── anthropic/                # implements domain/llm.Client
│   │   ├── off/                      # Open Food Facts client
│   │   ├── fdc/                      # USDA FDC client
│   │   ├── imagestore/               # filesystem image storage + imaging (resize/crop/encode)
│   │   └── jsonld/                   # schema.org Recipe parser
│   ├── transport/
│   │   └── http/
│   │       ├── router.go             # chi route registrations
│   │       ├── handlers/             # thin translation (one file per aggregate)
│   │       ├── middleware/           # auth, logging, recovery, cors
│   │       ├── sse/                  # server-sent events helper
│   │       └── errormap.go           # domain error → HTTP status + message_key
│   ├── config/                       # env loading + validation
│   ├── i18n/                         # generated error key constants from en.json
│   └── testhelper/                   # NewTestDB, HTTP fixtures, time freezer
├── db/migrations/                    # goose .sql files
└── static/                           # embedded SvelteKit build output
```

### Dependency rule

- `domain/*` imports from `domain/*` and stdlib only.
- `adapters/*` imports `domain/*` (implements ports) and stdlib + vendored libs.
- `transport/*` imports `domain/*`, `adapters/*`, and stdlib.
- `cmd/plantry` imports everything and wires it.

A `go vet` or architecture-test run in CI enforces this: the domain must not grep-match `adapters/` or `transport/`.

### Why hexagonal-lite and not full DDD

We do not model aggregates with private fields and rich behavior on every entity. The pattern is:

- **Aggregates** are plain structs with exported fields and zero methods (they're transport-friendly).
- **Services** (one per package, e.g. `component.Service`, `planner.Service`) hold all business logic, take repos as dependencies, and are the only thing handlers call.
- **Pure calculators** (e.g. `nutrition`, `shopping`) are plain functions. Stateless. No deps. Trivial to test.

This avoids OOP ceremony while still forcing unit-testable logic.

---

## 4. Data Model

### 4.1 Entity-relationship summary

```
ingredients 1─┬─n ingredient_portions
              └─n component_ingredients ─n 1 components
                                            │
                                            ├─n component_instructions
                                            ├─n component_tags
                                            └─n variant_group (optional)

weeks 1─n plates 1─n plate_components ─n 1 components
                1─0..1 plate_feedback

templates 1─n template_components ─n 1 components

time_slots 1─n plates

user_profile (singleton)
ai_conversations 1─n ai_messages
settings (key-value, tech only)
```

### 4.2 Full schema (DDL)

All tables use `INTEGER PRIMARY KEY` (SQLite rowid alias). Timestamps are `TEXT` in ISO-8601 UTC.

```sql
-- Ingredients --------------------------------------------------------------

CREATE TABLE ingredients (
    id              INTEGER PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    source          TEXT NOT NULL CHECK (source IN ('manual','off','fdc')),
    barcode         TEXT,
    off_id          TEXT,
    fdc_id          TEXT,
    image_path      TEXT,
    -- Essential macros per 100 g. Non-null; default 0 for manually-entered ingredients.
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
    name,
    content='ingredients',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

-- (FTS triggers use goose StatementBegin/End blocks; see migration file.)

CREATE TABLE ingredient_portions (
    id              INTEGER PRIMARY KEY,
    ingredient_id   INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    unit            TEXT NOT NULL,
    grams_per_unit  REAL NOT NULL CHECK (grams_per_unit > 0),
    UNIQUE (ingredient_id, unit)
);

-- Components (formerly "recipes") -----------------------------------------

CREATE TABLE variant_groups (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE components (
    id                  INTEGER PRIMARY KEY,
    name                TEXT NOT NULL,
    role                TEXT NOT NULL CHECK (role IN (
                            'main','side_starch','side_veg','side_protein',
                            'sauce','drink','dessert','standalone'
                        )),
    variant_group_id    INTEGER REFERENCES variant_groups(id) ON DELETE SET NULL,
    reference_portions  REAL NOT NULL DEFAULT 1 CHECK (reference_portions > 0),
    prep_minutes        INTEGER,
    cook_minutes        INTEGER,
    image_path          TEXT,
    notes               TEXT,
    last_cooked_at      TEXT,
    cook_count          INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX ix_components_role           ON components(role);
CREATE INDEX ix_components_variant_group  ON components(variant_group_id);
CREATE INDEX ix_components_last_cooked    ON components(last_cooked_at);

CREATE VIRTUAL TABLE components_fts USING fts5(
    name,
    content='components',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TABLE component_ingredients (
    id              INTEGER PRIMARY KEY,
    component_id    INTEGER NOT NULL REFERENCES components(id) ON DELETE CASCADE,
    ingredient_id   INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE RESTRICT,
    amount          REAL NOT NULL CHECK (amount > 0),
    unit            TEXT NOT NULL,          -- "g" | "ml" | custom portion unit
    grams           REAL NOT NULL CHECK (grams > 0),   -- resolved absolute grams
    sort_order      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_component_ingredients_component ON component_ingredients(component_id);

CREATE TABLE component_instructions (
    id              INTEGER PRIMARY KEY,
    component_id    INTEGER NOT NULL REFERENCES components(id) ON DELETE CASCADE,
    step_number     INTEGER NOT NULL,
    text            TEXT NOT NULL
);
CREATE INDEX ix_component_instructions_component ON component_instructions(component_id);

CREATE TABLE component_tags (
    component_id    INTEGER NOT NULL REFERENCES components(id) ON DELETE CASCADE,
    tag             TEXT NOT NULL,
    PRIMARY KEY (component_id, tag)
);

-- Planner ------------------------------------------------------------------

CREATE TABLE weeks (
    id          INTEGER PRIMARY KEY,
    year        INTEGER NOT NULL,
    week_number INTEGER NOT NULL CHECK (week_number BETWEEN 1 AND 53),
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (year, week_number)
);

CREATE TABLE time_slots (
    id          INTEGER PRIMARY KEY,
    name_key    TEXT NOT NULL,          -- i18n key: "slot.breakfast"
    icon        TEXT NOT NULL,          -- lucide icon name
    sort_order  INTEGER NOT NULL DEFAULT 0,
    active      INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE plates (
    id          INTEGER PRIMARY KEY,
    week_id     INTEGER NOT NULL REFERENCES weeks(id) ON DELETE CASCADE,
    day         INTEGER NOT NULL CHECK (day BETWEEN 0 AND 6),   -- 0=Mon
    slot_id     INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
    note        TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX ix_plates_week_day_slot ON plates(week_id, day, slot_id);

CREATE TABLE plate_components (
    id              INTEGER PRIMARY KEY,
    plate_id        INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
    component_id    INTEGER NOT NULL REFERENCES components(id) ON DELETE RESTRICT,
    portions        REAL NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_plate_components_plate ON plate_components(plate_id);

CREATE TABLE plate_feedback (
    plate_id    INTEGER PRIMARY KEY REFERENCES plates(id) ON DELETE CASCADE,
    status      TEXT NOT NULL CHECK (status IN ('cooked','skipped','loved','disliked')),
    note        TEXT,
    rated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Templates ----------------------------------------------------------------

CREATE TABLE templates (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE template_components (
    id              INTEGER PRIMARY KEY,
    template_id     INTEGER NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    component_id    INTEGER NOT NULL REFERENCES components(id) ON DELETE RESTRICT,
    portions        REAL NOT NULL DEFAULT 1 CHECK (portions > 0),
    sort_order      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_template_components_template ON template_components(template_id);

-- Profile, AI, Settings ----------------------------------------------------

CREATE TABLE user_profile (
    id                      INTEGER PRIMARY KEY CHECK (id = 1),  -- singleton
    kcal_target             REAL,
    protein_pct             REAL,
    fat_pct                 REAL,
    carbs_pct               REAL,
    dietary_restrictions    TEXT NOT NULL DEFAULT '[]',  -- JSON array of strings
    preferences             TEXT NOT NULL DEFAULT '{}',  -- JSON object, AI-managed
    system_prompt           TEXT,
    locale                  TEXT NOT NULL DEFAULT 'en',
    updated_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE ai_conversations (
    id          INTEGER PRIMARY KEY,
    week_id     INTEGER REFERENCES weeks(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE ai_messages (
    id              INTEGER PRIMARY KEY,
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('system','user','assistant','tool')),
    content         TEXT NOT NULL,   -- JSON: text, tool_calls, tool_results
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX ix_ai_messages_conversation ON ai_messages(conversation_id);

CREATE TABLE settings (
    key     TEXT PRIMARY KEY,
    value   TEXT NOT NULL
);
```

### 4.3 Why two JSON columns

`user_profile.dietary_restrictions` and `user_profile.preferences` are intentionally JSON:

- **`dietary_restrictions`** is a free-form list (vegetarian, gluten-free, no pork, no cilantro). Structured tables would overfit.
- **`preferences`** is AI-managed. The agent writes observations there (`"prefers_quick_weekday_meals": true`, `"dislikes": ["mushrooms"]`). The schema evolves as the agent learns. JSON is correct.

Everything else is relational.

### 4.4 What we dropped from the old schema

- `recipe_ingredients_alternatives` — replaced by variant components.
- `planning_entry_overrides` — replaced by plate-level component swaps.
- 20 micronutrient columns — replaced by 6 essential macros.
- `ingredients.off_barcode` vs `barcode` split — collapsed into `barcode` + `off_id`.
- `time_slots.active` stays, but default slots are NOT seeded — users define their own on first open of Settings.

---

## 5. Domain Model (Go)

One file per aggregate. Plain structs. Services hold logic.

### 5.1 Example: Component package

```go
// internal/domain/component/component.go
package component

type Role string

const (
    RoleMain         Role = "main"
    RoleSideStarch   Role = "side_starch"
    RoleSideVeg      Role = "side_veg"
    RoleSideProtein  Role = "side_protein"
    RoleSauce        Role = "sauce"
    RoleDrink        Role = "drink"
    RoleDessert      Role = "dessert"
    RoleStandalone   Role = "standalone"
)

type Component struct {
    ID                int64
    Name              string
    Role              Role
    VariantGroupID    *int64
    ReferencePortions float64
    PrepMinutes       *int
    CookMinutes       *int
    ImagePath         *string
    Notes             *string
    LastCookedAt      *time.Time
    CookCount         int
    Ingredients       []Ingredient
    Instructions      []Instruction
    Tags              []string
    CreatedAt         time.Time
    UpdatedAt         time.Time
}

type Ingredient struct {
    ID           int64
    IngredientID int64
    Amount       float64
    Unit         string
    Grams        float64   // resolved at save time
    SortOrder    int
}

type Instruction struct {
    ID         int64
    StepNumber int
    Text       string
}
```

```go
// internal/domain/component/repository.go
package component

type Repository interface {
    Create(ctx context.Context, c *Component) error
    Update(ctx context.Context, c *Component) error
    Delete(ctx context.Context, id int64) error
    Get(ctx context.Context, id int64) (*Component, error)
    List(ctx context.Context, q ListQuery) ([]Component, int, error)
    Siblings(ctx context.Context, variantGroupID int64) ([]Component, error)
    MarkCooked(ctx context.Context, id int64, at time.Time) error
}

type ListQuery struct {
    Search    string
    Role      *Role
    Tag       string
    Limit     int
    Offset    int
    SortBy    string   // "name" | "kcal" | "created" | "last_cooked"
    SortDesc  bool
}
```

```go
// internal/domain/component/service.go
package component

type Service struct {
    repo        Repository
    ingredients ingredient.Repository
    nutrition   nutrition.Calculator
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*Component, error) {
    // 1. Validate role
    // 2. Resolve each ingredient line to absolute grams via portions + unit
    // 3. Persist transactionally
    // 4. Return loaded component
}
```

Ports (interfaces) live in the domain. Adapters implement them. Handlers call services.

### 5.2 Nutrition calculator — the single source

```go
// internal/domain/nutrition/calculator.go
package nutrition

type Macros struct {
    Kcal    float64
    Protein float64
    Fat     float64
    Carbs   float64
    Fiber   float64
    Sodium  float64
}

type IngredientInput struct {
    Per100g Macros
    Grams   float64
}

func FromIngredients(inputs []IngredientInput) Macros { ... }

type ComponentInput struct {
    Ingredients       []IngredientInput
    ReferencePortions float64
}

func PerPortion(in ComponentInput) Macros { ... }

type PlateInput struct {
    Components []struct {
        ComponentMacros Macros   // per portion
        Portions        float64
    }
}

func PlateTotal(in PlateInput) Macros { ... }
```

Every nutrition number in the app — API response, shopping panel, AI tool output — flows through these three functions. No exceptions.

### 5.3 LLM port

```go
// internal/domain/llm/client.go
package llm

type Message struct {
    Role       string         // "system" | "user" | "assistant" | "tool"
    Content    string
    ToolCalls  []ToolCall
    ToolCallID string         // for tool role
}

type ToolCall struct {
    ID    string
    Name  string
    Input json.RawMessage
}

type Tool struct {
    Name        string
    Description string
    Schema      json.RawMessage   // JSON schema for inputs
}

type Response struct {
    Message  Message
    Finish   string   // "stop" | "tool_use" | "length"
}

type Client interface {
    Complete(ctx context.Context, req Request) (*Response, error)
    Stream(ctx context.Context, req Request, out chan<- Event) error
}

type Request struct {
    Model       string
    System      string
    Messages    []Message
    Tools       []Tool
    Temperature float64
    MaxTokens   int
}
```

OpenAI and Anthropic adapters implement `Client`. Neither leaks into the domain. Agent orchestration (the tool-calling loop) lives in `domain/planner/agent.go` and is provider-agnostic.

---

## 6. REST API

All endpoints are under `/api`. All responses are JSON with a `Content-Type: application/json` header. All error responses have shape `{"message_key": "error.xxx", "status": 4xx}`.

### 6.1 Conventions

- `GET` lists accept `?search=&limit=&offset=&sort=&order=asc|desc`.
- `POST` returns `201 Created` with the full resource.
- `PUT` returns `200 OK` with the full resource.
- `DELETE` returns `204 No Content`.
- Path parameters: `{id}` for numeric IDs.
- Query parameters are always lowercase snake_case.
- ISO-8601 UTC for all timestamps.
- All write endpoints require a `Content-Type: application/json` header.

### 6.2 Endpoint catalogue

#### Health

```
GET    /api/health                                  → { status: "ok" }
```

#### Ingredients

```
GET    /api/ingredients                             list (search, sort, paginate)
POST   /api/ingredients                             create
GET    /api/ingredients/{id}                        detail + portions
PUT    /api/ingredients/{id}                        update
DELETE /api/ingredients/{id}                        delete (409 if referenced)

POST   /api/ingredients/{id}/image                  multipart upload
DELETE /api/ingredients/{id}/image                  remove image

GET    /api/ingredients/lookup?barcode=…            proxy to OFF/FDC
GET    /api/ingredients/lookup?q=…                  proxy to OFF/FDC
POST   /api/ingredients/resolve                     body: {name} → best matches

GET    /api/ingredients/{id}/portions               list portion units
POST   /api/ingredients/{id}/portions               upsert unit → grams
DELETE /api/ingredients/{id}/portions/{unit}        remove unit
```

#### Components

```
GET    /api/components                              list (search, role, tag, sort)
POST   /api/components                              create
GET    /api/components/{id}                         full detail (ingredients, steps, tags, variants)
PUT    /api/components/{id}                         update
DELETE /api/components/{id}                         delete (409 if used in plates)

POST   /api/components/{id}/image                   multipart upload
DELETE /api/components/{id}/image                   remove

POST   /api/components/{id}/variant                 create new component in same variant group
GET    /api/components/{id}/variants                list siblings in variant group

GET    /api/components/{id}/nutrition               per-portion macros
```

#### Weeks and plates

```
GET    /api/weeks                                   paginated list (archive)
GET    /api/weeks/current                           get-or-create current week
GET    /api/weeks/by-date?year=&week=               get-or-create specific
GET    /api/weeks/{id}                              week with all plates + components
POST   /api/weeks/{id}/copy                         body: {target_year, target_week}

POST   /api/weeks/{id}/plates                       body: {day, slot_id, components?: [...]}
GET    /api/plates/{id}                             detail
PUT    /api/plates/{id}                             update note, day/slot move
DELETE /api/plates/{id}                             remove plate

POST   /api/plates/{id}/components                  body: {component_id, portions}
PUT    /api/plates/{id}/components/{pcId}           body: {portions?, component_id?} (swap or rescale)
DELETE /api/plates/{id}/components/{pcId}           remove component from plate

POST   /api/plates/{id}/feedback                    body: {status, note?}
```

#### Shopping and nutrition

```
GET    /api/weeks/{id}/shopping-list                aggregated list (grams per ingredient)
GET    /api/weeks/{id}/nutrition                    day + week totals, delta vs targets
```

#### Templates

```
GET    /api/templates                               list
POST   /api/templates                               create (optional from_plate_id)
GET    /api/templates/{id}                          detail
PUT    /api/templates/{id}                          update
DELETE /api/templates/{id}                          delete
POST   /api/templates/{id}/apply                    body: {plate_id} → fills plate
```

#### Profile and settings

```
GET    /api/profile                                 read singleton
PUT    /api/profile                                 update (targets, restrictions, system_prompt, locale)

GET    /api/settings                                all tech settings
PUT    /api/settings/{key}                          update one (body: {value})
GET    /api/settings/slots                          time slots
POST   /api/settings/slots                          create
PUT    /api/settings/slots/{id}                     update
DELETE /api/settings/slots/{id}                     delete (409 if used by plates)
GET    /api/settings/ai/models                      provider model list (proxy)
```

#### AI

```
POST   /api/ai/chat                                 SSE stream
                                                    body: {week_id, mode, messages}
                                                    events: assistant, tool_call, tool_result,
                                                            plate_changed, done, error

GET    /api/ai/conversations?week_id=               list (metadata only)
GET    /api/ai/conversations/{id}                   full history
DELETE /api/ai/conversations/{id}                   delete
```

#### Import

```
POST   /api/import/extract                          body: {url} → draft component JSON
POST   /api/import/resolve                          body: {draft} → resolved component ready to save
```

#### Images (static)

```
GET    /images/{type}/{id}.{ext}                    filesystem-backed, type ∈ {ingredients, components}
```

### 6.3 Error key catalogue

Error keys live in `frontend/src/lib/locales/{en,de}.json` under the `error.` namespace and are generated into Go constants (`internal/i18n/errors.go`) by a `go generate` step in CI. Sample:

```
error.invalid_body
error.invalid_id
error.not_found
error.server
error.ingredient.duplicate_name
error.ingredient.in_use
error.ingredient.lookup_failed
error.component.invalid_role
error.component.in_use
error.plate.slot_unknown
error.plate.component_role_missing
error.plate.feedback_invalid_status
error.template.empty
error.import.fetch_failed
error.import.parse_failed
error.import.ai_unavailable
error.ai.provider_missing
error.ai.tool_failed
error.ai.stream_interrupted
error.profile.invalid_macros
```

The generator ensures every key referenced in Go exists in both locale files. CI fails if a key is missing or orphaned.

---

## 7. AI Agent Architecture

### 7.1 Tool schema (domain-defined)

```
list_components(role?, tag?, search?, max_cook_minutes?, limit?)
get_component(component_id)
get_week(week_id)
get_week_nutrition(week_id)
get_profile()
create_plate(week_id, day, slot_id)
add_component_to_plate(plate_id, component_id, portions)
swap_component(plate_component_id, new_component_id, portions?)
update_plate_component(plate_component_id, portions)
remove_plate_component(plate_component_id)
delete_plate(plate_id)
clear_week(week_id)                            -- bulk op for "replace_all" mode
record_preference(key, value)                  -- AI writes to profile.preferences
```

Each tool is defined once in `domain/llm/tools.go` with a JSON schema and a handler function of type `func(context.Context, json.RawMessage) (json.RawMessage, error)`. Handlers call domain services; they never touch SQL directly. This gives a single execution point for tests and for SSE event emission.

### 7.2 Tool loop

```
loop:
    response = llm.Complete(req with current messages)
    if response.Finish == "stop":
        emit assistant final; break
    for call in response.Message.ToolCalls:
        result = tools[call.Name](call.Input)
        emit tool_call event
        emit tool_result event
        append tool result to messages
        if call caused plate change:
            emit plate_changed event so frontend refreshes
    iterations++
    if iterations > 20: emit error; break
```

All emissions go through the SSE helper. The frontend listens and patches local state.

### 7.3 System prompt composition

Built on every turn:

1. Plantry role description (static).
2. User profile JSON (targets, restrictions, preferences, custom system_prompt).
3. Week context (current plates summary).
4. Tool usage rules.

Capped at 4k tokens. Rebuild every turn (cheap; keeps context fresh).

---

## 8. Frontend Architecture

### 8.1 Directory layout

```text
frontend/
├── public/                            # static assets
├── index.html
├── vite.config.ts
├── tailwind.config.ts
├── tsconfig.json
└── src/
    ├── main.tsx                       # React root: QueryClient + RouterProvider + I18nProvider
    ├── styles/
    │   └── globals.css                # Tailwind + OKLch tokens
    ├── routes/                        # TanStack Router file-based routes
    │   ├── __root.tsx                 # shell, nav, toast host, error boundary
    │   ├── index.tsx                  # planner (home, current week)
    │   ├── archive/
    │   │   ├── index.tsx
    │   │   └── $id.tsx
    │   ├── components/
    │   │   ├── index.tsx
    │   │   ├── new.tsx
    │   │   └── $id/
    │   │       ├── index.tsx
    │   │       └── edit.tsx
    │   ├── ingredients/
    │   ├── templates/
    │   ├── import/
    │   └── settings/
    └── lib/
        ├── domain/                    # PURE TypeScript types + calculators
        │   ├── types.ts               # Component, Plate, Ingredient, Role, etc.
        │   ├── nutrition.ts           # mirror of Go calculator, pure functions
        │   ├── roles.ts
        │   └── units.ts
        ├── api/                       # raw typed HTTP client (used only by queries/)
        │   ├── client.ts              # fetch wrapper, error mapping
        │   ├── ingredients.ts
        │   ├── components.ts
        │   ├── plates.ts
        │   ├── weeks.ts
        │   ├── templates.ts
        │   ├── profile.ts
        │   ├── ai.ts                  # SSE stream reader
        │   └── index.ts
        ├── queries/                   # TanStack Query hooks (server state)
        │   ├── keys.ts                # centralized query-key factory
        │   ├── ingredients.ts         # useIngredients, useCreateIngredient, ...
        │   ├── components.ts
        │   ├── weeks.ts
        │   ├── plates.ts              # useSwapComponent w/ optimistic update
        │   ├── templates.ts
        │   ├── profile.ts
        │   └── ai.ts                  # useChatStream
        ├── stores/                    # Zustand stores (ephemeral UI state only)
        │   ├── planner-ui.ts          # edited plate, drag state, panel visibility
        │   ├── chat-ui.ts             # draft message, panel visibility
        │   └── toast.ts
        ├── components/
        │   ├── ui/                    # shadcn/ui primitives
        │   ├── planner/               # PlannerGrid, PlateCell, PlateEditor, ComponentChip
        │   ├── component/             # ComponentEditor, ComponentPicker, VariantList
        │   ├── ingredient/            # IngredientEditor, LookupPanel, BarcodeScanner
        │   ├── chat/                  # ChatPanel, ChatMessage, ToolCallBlock
        │   ├── shopping/              # ShoppingPanel
        │   ├── nutrition/             # MacroBar, DaySummary, WeekSummary
        │   └── shared/                # EmptyState, PageHeader, FormBreadcrumb
        ├── hooks/                     # small reusable hooks
        ├── i18n/
        │   ├── index.ts               # react-i18next init
        │   ├── en.json
        │   └── de.json
        └── utils/                     # cn(), date helpers, formatters
```

### 8.2 State philosophy — three buckets, strict boundaries

**Server state** lives in TanStack Query. No exceptions. Anything that came from the API or will be written to it (components, ingredients, plates, weeks, shopping list, nutrition, profile, AI conversations) is a Query or Mutation. Cache keys are centralized in `lib/queries/keys.ts` so two hooks never drift apart.

**Client state** lives in small Zustand stores. Only genuinely client-only UI state: which panel is open, which plate is being edited, draft chat message text, in-progress drag state. If a piece of state would need to be synced across tabs or persisted server-side, it belongs in TanStack Query instead.

**Derived state** is computed inline with `useMemo` or via TanStack Query's `select` option. No reactive libraries, no signals, no global event bus.

This three-bucket discipline is enforced in code review: a new `useState` holding anything fetched from the API is wrong — it should be a query hook. A new Zustand store that mirrors server data is wrong — delete it and use a query.

### 8.3 TanStack Query patterns

**Query keys** come from a single factory to prevent drift:

```ts
// lib/queries/keys.ts
export const qk = {
  components: {
    all: ['components'] as const,
    list: (q: ListQuery) => ['components', 'list', q] as const,
    detail: (id: number) => ['components', 'detail', id] as const,
    variants: (id: number) => ['components', 'variants', id] as const,
  },
  weeks: {
    current: ['weeks', 'current'] as const,
    byId: (id: number) => ['weeks', id] as const,
    shoppingList: (id: number) => ['weeks', id, 'shopping-list'] as const,
    nutrition: (id: number) => ['weeks', id, 'nutrition'] as const,
  },
  profile: ['profile'] as const,
  ai: {
    conversation: (id: number) => ['ai', 'conversation', id] as const,
  },
};
```

**Optimistic updates** follow the canonical pattern. Every mutation in the planner uses the same shape — cancel in-flight queries, snapshot previous data, apply patch, roll back on error, invalidate on settle:

```ts
useMutation({
  mutationFn: ({ pcId, componentId }) =>
    api.plates.swapComponent(plateId, pcId, componentId),
  onMutate: async ({ pcId, componentId }) => {
    await queryClient.cancelQueries({ queryKey: qk.weeks.byId(weekId) });
    const previous = queryClient.getQueryData(qk.weeks.byId(weekId));
    queryClient.setQueryData(qk.weeks.byId(weekId), (old) =>
      patchPlateComponent(old, pcId, componentId)
    );
    return { previous };
  },
  onError: (_err, _vars, ctx) => {
    queryClient.setQueryData(qk.weeks.byId(weekId), ctx?.previous);
  },
  onSettled: () => {
    queryClient.invalidateQueries({ queryKey: qk.weeks.byId(weekId) });
    queryClient.invalidateQueries({ queryKey: qk.weeks.shoppingList(weekId) });
    queryClient.invalidateQueries({ queryKey: qk.weeks.nutrition(weekId) });
  },
});
```

This replaces the hand-rolled optimistic state class that a Svelte design would need. Write the mutation hook once; every caller gets rollback for free.

**AI streaming** uses a custom hook that owns the SSE reader and writes into the React Query cache on `plate_changed` events:

```ts
useChatStream({
  weekId,
  onPlateChanged: () => {
    queryClient.invalidateQueries({ queryKey: qk.weeks.byId(weekId) });
    queryClient.invalidateQueries({ queryKey: qk.weeks.nutrition(weekId) });
    queryClient.invalidateQueries({ queryKey: qk.weeks.shoppingList(weekId) });
  },
});
```

### 8.4 Nutrition math on the frontend

Frontend pulls numbers from the API as the single source of truth. The `lib/domain/nutrition.ts` module exists only as a pure TypeScript mirror for live-preview situations (e.g. showing macros while editing a component before save). It must stay in lock-step with the Go calculator via a shared fixture file: a Vitest test reads `testdata/nutrition-cases.json` and a Go test reads the same file. CI fails on divergence.

### 8.5 Routes and flows

TanStack Router file-based routes with typed params (`$id` segments):

| Route                        | Purpose                                              |
| ---------------------------- | ---------------------------------------------------- |
| `/`                          | Current week planner (primary screen)                |
| `/archive`                   | Paginated past weeks                                 |
| `/archive/$id`               | Read-only week view                                  |
| `/components`                | Component library (list, search, role filter)        |
| `/components/new`            | Create component                                     |
| `/components/$id`            | Detail + variants + nutrition                        |
| `/components/$id/edit`       | Edit component                                       |
| `/ingredients`               | Ingredient catalog                                   |
| `/ingredients/new`           | Create ingredient (or via lookup)                    |
| `/ingredients/$id/edit`      | Edit ingredient                                      |
| `/templates`                 | Template library                                     |
| `/import`                    | Recipe import wizard                                 |
| `/settings`                  | Profile, targets, time slots, API keys, AI config    |

### 8.6 Forms

Every form uses `react-hook-form` with a `zod` resolver. Schemas are colocated with the form and mirror the shape of the API payload — one type definition, automatic client-side validation, clean integration with shadcn/ui inputs. Server validation errors (4xx with a `message_key`) are surfaced into the form via `setError` so the user sees them inline, not only as a toast.

### 8.7 Drag-and-drop

The planner grid uses `dnd-kit`: move plates between day/slot cells, reorder components inside a plate. `dnd-kit` handles keyboard accessibility out of the box (Tab + Space to grab, arrow keys to move, Escape to cancel), which matters for a tool people use daily. Touch support is built in for mobile.

### 8.8 Barcode scanning

The `BarcodeScanner` component uses the native `BarcodeDetector` API when available (Chromium-based browsers) and falls back to `@zxing/library` on Firefox and Safari. The fallback is lazy-loaded (`import()` on first camera open) so desktop Chromium users never pay its ~100 KB cost. Both paths expose the same callback interface.

### 8.9 Accessibility

- shadcn/ui primitives ship with correct ARIA by default.
- Every interactive element is keyboard-reachable; focus rings always visible.
- Color contrast meets WCAG AA on both themes.
- e2e tests include keyboard-only flows for the planner and chat.
- `dnd-kit` keyboard sensors are enabled on all drag surfaces.

---

## 9. Cross-Cutting Concerns

### 9.1 Errors

All domain errors are sentinel values:

```go
var (
    ErrNotFound          = errors.New("not found")
    ErrDuplicateName     = errors.New("duplicate name")
    ErrInUse             = errors.New("in use")
    ErrInvalidRole       = errors.New("invalid role")
    ...
)
```

Handlers map them via a single `errormap.go` function:

```go
func toHTTP(err error) (status int, messageKey string) {
    switch {
    case errors.Is(err, ErrNotFound):
        return 404, "error.not_found"
    case errors.Is(err, ErrDuplicateName):
        return 409, "error.ingredient.duplicate_name"
    ...
    default:
        return 500, "error.server"
    }
}
```

No string matching on error messages we own.

### 9.2 Transactions

Every multi-row mutation wraps in a single `sqlite.Tx`:

- Create component (component + ingredients + instructions + tags)
- Update component (replace children)
- Copy week (plates + plate_components)
- Apply template to plate
- Delete cascade (FK handles the rest)

Repositories expose both `Create` (auto-tx) and `CreateTx(tx, ...)` variants. Services compose multi-aggregate work through the `sqlite.TxRunner` helper.

### 9.3 Logging

`log/slog` JSON handler. Request middleware logs method, path, status, duration, and a request ID. Error logs include stack info for unexpected errors only.

### 9.4 Auth

Optional. If `PLANTRY_AUTH_PASSWORD` is set, a middleware requires either a session cookie (set via `POST /api/auth/login`) or a `X-Plantry-Password` header. No user system; a single shared password is the household model. Sessions are signed cookies with HMAC, no server-side store.

### 9.5 Rate limiting

Token bucket on the LLM chat endpoint (default 10 req/min per IP) to prevent runaway costs. Configurable via env.

### 9.6 Observability

- `/api/health` returns `{status, version, db_ok}`.
- Request logs via slog.
- Optional Prometheus endpoint `/metrics` gated by env flag (deferred past v1 unless trivial).

### 9.7 i18n

- `react-i18next`, two locale files (en, de).
- Every `message_key` in the API must exist in both.
- Go constants generated from `en.json` to catch typos at compile time.
- `name_key` pattern for time slots (`slot.breakfast`, etc.).

### 9.8 Testing strategy

| Layer              | Tool            | Scope                                                  |
| ------------------ | --------------- | ------------------------------------------------------ |
| Domain services    | `testing`       | Pure logic, no DB                                      |
| Pure calculators   | `testing`       | Table-driven, every edge case                          |
| SQLite adapters    | `testing` + in-memory DB | Migration round-trip, FTS triggers, constraints |
| HTTP handlers      | `httptest`      | Status codes, message keys, happy + error paths        |
| LLM adapters       | `testing` + httptest fake upstream | Wire format, tool call parsing          |
| Frontend pure      | Vitest          | Nutrition calc, unit conversions, SSE parser           |
| React components   | Vitest + React Testing Library | Critical components (PlateEditor, ComponentPicker) |
| React hooks        | Vitest + `@testing-library/react` | Query hooks, mutation optimistic rollback paths  |
| End-to-end         | Playwright      | Every user-facing feature, per phase                   |

Rule: no feature ships without at least one domain test, one adapter test (if it touches adapters), and one e2e test.

---

## 10. Operational Model

- Single Docker container, single SQLite file, single images directory.
- Users deploy with `docker-compose up`; persistent volume mounted at `/data`.
- Upgrades: pull new image, restart — goose runs pending migrations on startup.
- Backup: stop container → copy `/data` → start container. Or use SQLite `.backup` command via a mounted cron job.
- No blue-green, no rollback tooling; SQLite backup is the recovery story.
- Minimum hardware: Raspberry Pi 3 B+ (1 GB RAM). Target memory footprint under 128 MB resident.

---

## 11. What's explicitly NOT in the architecture

- Event sourcing (overkill for a single-user planner).
- CQRS (same).
- GraphQL (REST is sufficient and cheaper to test).
- WebSockets (SSE is enough for one-directional streaming).
- Multi-user / roles / RBAC (one household, one password).
- Offline-first sync / CRDTs (one client, one server, LAN-local).
- Feature flags (ship features directly via phases).
- Pantry tracking (deliberate omission; without physical sync, it's noise).
- Per-ingredient swap overrides (replaced by plate-level component swaps and variant components).
