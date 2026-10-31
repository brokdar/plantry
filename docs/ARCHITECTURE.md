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

### Colors

The "Cookbook" palette (paper-white ground, ink-blue accent, six batch colours) with Newsreader headings and Source Sans 3 for UI. Tokens and values: `docs/UX_DESIGN_SPEC.md` §3. Implementation: `frontend/src/index.css`, where the older shadcn and Material token names are aliased onto the new ones.

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
PLANTRY_SECRET_KEY=             # optional; default: generated into /data/secret.key
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
│   │   ├── food/                     # Food aggregate (leaf | composed), nutrition resolver, history
│   │   ├── plate/                    # plates, components, cook sessions, snapshots, events
│   │   ├── planview/                 # read models: plan and today
│   │   ├── window/                   # plan window arithmetic
│   │   ├── nutrition/                # Macros value type
│   │   ├── shopping/                 # shopping list derivation + section classifier
│   │   ├── units/                    # unit conversion and display rounding
│   │   ├── feedback/                 # plate feedback
│   │   ├── profile/                  # user profile aggregate + targets
│   │   ├── preset/                   # saved day / week compositions
│   │   ├── slot/, availability/      # time slots, cell modes
│   │   ├── settings/, setup/         # settings registry, first-run status
│   │   ├── actor/                    # user | agent, carried in ctx
│   │   ├── llm/                      # LLM port interface + tool schema types
│   │   ├── importer/                 # URL → draft recipe port + orchestration
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
- **Services** (one per package, e.g. `food.Service`, `plate.Service`) hold all business logic, take repos as dependencies, and are the only thing handlers call.
- **Pure calculators** (e.g. `nutrition`, `shopping`) are plain functions. Stateless. No deps. Trivial to test.

This avoids OOP ceremony while still forcing unit-testable logic.

---

## 4. Data Model

SQLite, one file. Numbered goose migrations in `backend/db/migrations/` (SQL, plus Go migrations for data moves and table rebuilds); `backend/db/schema_patches/` gives sqlc the end-state schema where a Go migration changes it. Timestamps are `TEXT` (`datetime('now')`); dates are `YYYY-MM-DD` text.

### 4.1 Entity-relationship summary

```
variant_groups 1─n foods
foods (kind = leaf | composed)
  ├─n food_portions        (unit → grams)
  ├─n food_components      (composed parent → child food, amount, unit, grams)
  ├─n food_instructions
  └─n food_tags

time_slots 1─n plates 1─n plate_components ─n 1 foods
                  │                 └─0..1 cook_sessions (composed components only)
                  └─0..1 plate_feedback
cook_sessions n─1 foods, n─1 time_slots (cook cell)

presets 1─n preset_plates 1─n preset_components ─n 1 foods
presets 1─n preset_tags

availability_defaults (weekday × slot), availability_overrides (date × slot) ─0..1 presets
plate_events          (audit trail, no foreign keys)

user_profile (singleton)
app_settings (key-value)
ai_conversations 1─n ai_messages
ai_runs 1─n ai_run_steps;  ai_runs ─0..1 ai_prompts;  ai_tool_schemas
```

### 4.2 Tables

The authoritative DDL is the migrations; this is the shape as of migration 00033.

| Table | Purpose and key facts |
|---|---|
| `foods` | One table for both kinds. `kind = 'leaf'` has a `source` (`manual` / `off` / `fdc`), per-100 g macros and micros, and an optional `shop_section` (00031; leaves only: `produce`, `bakery`, `dairy_eggs`, `meat_fish`, `dry_goods`, `cans_jars`, `spices_oils`, `frozen`, `other`). `kind = 'composed'` (a recipe) has `reference_portions`, `role`, `prep_minutes`, `cook_minutes`, `keep_days` (1-7), `freezable`, `source_url`, `description`, `variant_group_id`. Both have `image_path`, `favorite`, `notes`. CHECKs enforce the kind-specific columns. |
| `food_components` | A composed food's ingredients: `parent_id`, `child_id` (another food), `amount`, `unit`, `grams`, `sort_order`. |
| `food_portions` | Per-food unit-to-grams table, `(food_id, unit)`. |
| `food_instructions`, `food_tags`, `variant_groups` | Steps, tags, and groups of variant recipes. |
| `time_slots` | `name_key`, `icon`, `sort_order`, `active`. Not seeded; the user defines them. |
| `plates` | One plate per cell: `date`, `slot_id`, `eaters` (REAL, > 0), `note`. `(date, slot_id)` is unique; `AUTOINCREMENT` since 00030. |
| `plate_components` | Either a composed component (`portions` per eater, `cook_session_id` required) or a leaf (`amount`, `unit`, `grams`, `grams_source`, no session). `sort_order` orders them. A CHECK enforces the two shapes. |
| `cook_sessions` | "Food X is cooked at (`cook_date`, `cook_slot_id`) producing `portions`", with `frozen_portions` (<= portions), `cooked_at`, `note`. `AUTOINCREMENT`. Plate components eat from it. |
| `plate_feedback` | `plate_id` primary key, `status` in `loved` / `disliked` / `not_eaten`, `note`. |
| `plate_events` | Audit trail (D20): `plate_id`, `cook_session_id`, `date`, `slot_id`, `action`, `actor` (`user` / `agent`), `ai_run_id`, `before_json`, `after_json`, `at`. No foreign keys, so rows outlive what they describe. |
| `presets`, `preset_plates`, `preset_components`, `preset_tags` | Saved day or week compositions by slot; `presets.id` is `AUTOINCREMENT` (00033). Components hold a quantity like plate components, without sessions. |
| `availability_defaults`, `availability_overrides` | Cell mode `plan` / `routine` / `none`; routine needs a `preset_id`. Defaults are by weekday (0 = Monday) and slot, overrides by date and slot. |
| `user_profile` | Singleton: macro targets, `dietary_restrictions` (JSON list), `preferences` (JSON), `system_prompt`, `locale`, `diet_type`, `meat_days_per_week_max`, `household_portions`. |
| `app_settings` | Key-value with an `encrypted` flag for API keys. Registry in `domain/settings/specs.go`. |
| `ai_conversations`, `ai_messages` | Chat history (conversations carry no week reference). |
| `ai_runs`, `ai_run_steps`, `ai_prompts`, `ai_tool_schemas` | Metered LLM log; see `backend/CLAUDE.md`. |

Derived data is not stored: a food's last-eaten date, times cooked and used-in count come from plates and cook sessions at read time (D16), and the shopping list and plan views are computed from plates and sessions.

### 4.3 Why two JSON columns

`user_profile.dietary_restrictions` and `user_profile.preferences` are intentionally JSON:

- **`dietary_restrictions`** is a free-form list (vegetarian, gluten-free, no pork, no cilantro). Structured tables would overfit.
- **`preferences`** is AI-managed. The agent writes observations there (`"prefers_quick_weekday_meals": true`, `"dislikes": ["mushrooms"]`). The schema evolves as the agent learns. JSON is correct.

Everything else is relational.

### 4.4 Ids and table rebuilds

`plates`, `cook_sessions` and `presets` use `AUTOINCREMENT` so an id never names two rows over time (a stale undo payload or routine then names nothing). A table rebuild runs as a Go no-transaction migration with foreign keys off on a pinned connection and a `foreign_key_check` before commit, so a drop never cascades into dependants (see `backend/CLAUDE.md`).

---

## 5. Domain Model (Go)

One package per aggregate under `backend/internal/domain/`. Plain structs; services hold the logic and take repository interfaces. `backend/CLAUDE.md` has the details.

### 5.1 Packages

| Package | Role |
|---|---|
| `food` | The Food aggregate (leaf or composed), recipe trees (`LoadTrees`), per-portion nutrition (`NutritionResolver`), derived history (`Derive`), shop sections |
| `plate` | Plates, components, cook sessions and their batch rules; the single write path (`plate.Service`), snapshots (`PlanSnapshot`, `Apply`), audit events |
| `planview` | Read models: `Plan` (`GET /api/plan`) and `Today` (`GET /api/today`); never writes |
| `window` | Pure plan-window arithmetic: a window is 7 days starting on `plan.shopping_day` |
| `shopping` | Shopping list from cook sessions and leaf components, sorted into store sections |
| `availability`, `slot`, `preset` | Cell modes, time slots, saved compositions |
| `nutrition` | The `Macros` value type |
| `units` | Unit normalisation, count-to-grams, display rounding |
| `settings`, `profile`, `setup` | Settings registry, user profile, first-run status |
| `feedback` | Plate feedback |
| `agent`, `llm`, `airun`, `importer` | AI chat agent and tools, LLM port, run log, recipe import |
| `actor` | Who made a change (`user` / `agent`), carried in ctx |

### 5.2 Core rules

**Unit of work (D21).** `domain.Transactor.InTx(ctx, fn)` runs `fn` in one transaction; every repository called with the ctx joins it, and a nested `InTx` joins the outer one. Multi-row mutations (plate and session changes, preset apply, availability `none` with plate removal) use it.

**Plate aggregate.** A cell holds at most one plate. A plate has `eaters` and ordered components; a composed component eats `portions` per eater from a cook session, a leaf component has `amount` + `unit`. All writes go through `plate.Service`, which returns a `PlanSnapshot` for deletes (restored by `POST /api/plan/restore` or `Apply`) and writes plate events last, in the same unit of work.

**Cook sessions and D11.** A session links to its components as fresh, leftover or frozen, in cell order. When a change alters what the links consume, a session that exactly covered them still does; a session with a surplus or deficit keeps its portions. Sessions left without links are deleted.

**Nutrition.** One source: `food.NutritionResolver` computes per-portion macros (`PerPortion`, `PerPortionDetailed` with a completeness flag and the leaves lacking core macros, and the batched `PerPortionDetailedMany`) from the leaves' per-100 g values and `food_components`, recursively, divided by `reference_portions`. Plate and day totals in `planview` multiply those by portions and eaters. Every nutrition number in the API, the AI tools included, flows through it; `nutrition.Macros` is the shared value type.

**Rounding.** Amounts are stored and summed exact; `units.Display` rounds for reading and `units.Buy` rounds count units up for shopping.

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

All endpoints are under `/api`. Responses are JSON (`Content-Type: application/json`), except the SSE chat stream. Error responses have shape `{"message_key": "error.xxx", "status": 4xx}`. Routes are registered in `backend/internal/transport/http/router.go`.

### 6.1 Conventions

- `GET /api/foods` and `GET /api/ai/conversations` accept `limit` and `offset`; foods also take the filters below.
- `POST` creating a resource returns `201 Created` with the full resource.
- `PUT` returns `200 OK` with the full resource.
- `DELETE` returns `204 No Content`, except plate, session and plan deletes, which return the `PlanSnapshot` that restores them.
- Path parameters: `{id}` for numeric IDs. Dates are `YYYY-MM-DD`.
- Query parameters are lowercase snake_case. Read models take an optional `today` (default: the server's local date).
- Timestamps are ISO-8601 UTC.
- Write endpoints require a `Content-Type: application/json` header.

### 6.2 Endpoint catalogue

#### Health and units

```
GET    /api/health
GET    /api/units
```

#### Foods (leaves and recipes)

```
GET    /api/foods                                   ?search&kind&role&tag&favorite=1&sort=name|last_eaten|created&order
                                                    &max_minutes&not_eaten_weeks&created_within_days&today&limit&offset
POST   /api/foods                                   create (leaf or composed)
GET    /api/foods/macros                            ?ids= batch per-portion macros
GET    /api/foods/lookup                            leaf lookup (OFF / FDC / AI)
POST   /api/foods/resolve                           pick a lookup candidate
GET    /api/foods/{id}
PUT    /api/foods/{id}
DELETE /api/foods/{id}                              409 if in use
GET    /api/foods/{id}/nutrition                    per-portion macros
GET    /api/foods/{id}/scaled?portions=             scaled ingredient list (0 < portions <= 100)
POST   /api/foods/{id}/favorite
PUT    /api/foods/{id}/shop-section                 leaves only
POST   /api/foods/{id}/variant                      create a variant recipe
GET    /api/foods/{id}/variants
GET    /api/foods/{id}/portions
POST   /api/foods/{id}/portions                     upsert a unit → grams
DELETE /api/foods/{id}/portions/{unit}
POST   /api/foods/{id}/sync-portions
POST   /api/foods/{id}/refetch                      re-run the leaf lookup
POST   /api/foods/{id}/image                        upload
DELETE /api/foods/{id}/image
POST   /api/image/fetch-url                         fetch a remote image
```

Recipes carry `last_eaten`, `times_cooked` and `per_portion`; leaves carry `used_in_count` (derived, D16).

#### Plan, plates and cook sessions

```
GET    /api/plan?start&today                        the 7-day window: cells, sessions, days, targets, stats
DELETE /api/plan?from&to[&slot_id]                  clear plates; returns the restoring PlanSnapshot
POST   /api/plan/restore                            write a PlanSnapshot, all or nothing (409 error.plan.cell_occupied)
GET    /api/today?date&today                        verdict, cook, meals, reminders, rate, plan banner

POST   /api/plates                                  create (date, slot, eaters, components)
GET    /api/plates/{id}
PUT    /api/plates/{id}                             date, slot, eaters, note
DELETE /api/plates/{id}                             returns the restoring PlanSnapshot
POST   /api/plates/{id}/components
PUT    /api/plates/{id}/components/{pcId}           change quantity or swap food
DELETE /api/plates/{id}/components/{pcId}
PUT    /api/plates/{id}/feedback                    loved | disliked | not_eaten (+ note)
DELETE /api/plates/{id}/feedback

GET    /api/cook-sessions/{id}
PUT    /api/cook-sessions/{id}                      portions, frozen portions, note
DELETE /api/cook-sessions/{id}                      returns the restoring PlanSnapshot
POST   /api/cook-sessions/{id}/cooked               mark (un)cooked
GET    /api/cook-sessions/{id}/leftover-cells       ?count=
POST   /api/cook-sessions/{id}/leftovers            add leftover plates
```

There is no list-plates endpoint: `GET /api/plates`, `/api/plates/by-date`, `/api/plates/macros`, `/api/nutrition` and `/api/foods/{id}/recent-unit` were removed (the plan view carries that data); `TestLegacyReadRoutesGone` pins it.

#### Shopping

```
GET    /api/shopping-list                           ?from&to&today — sections in walk order; default range: the anchored window's
```

#### Presets

```
GET    /api/presets
GET    /api/presets/known-tags
POST   /api/presets
POST   /api/presets/copy-week                       copy a window's plates; returns created ids and what undoes it
GET    /api/presets/{id}
PUT    /api/presets/{id}
PATCH  /api/presets/{id}
DELETE /api/presets/{id}
POST   /api/presets/{id}/duplicate
POST   /api/presets/{id}/apply                      builds a snapshot and applies it (decide-all-then-write)
```

#### Profile and settings

```
GET    /api/profile                                 read singleton
PUT    /api/profile                                 update (targets, restrictions, system_prompt, locale, diet, household portions)

GET    /api/settings/slots                          time slots
POST   /api/settings/slots                          create
PUT    /api/settings/slots/{id}                     update
DELETE /api/settings/slots/{id}                     delete (409 if used by plates)
GET    /api/settings                                all keys with value and source (db / env / default)
PUT    /api/settings/{key}                          body: {value} — DB override
DELETE /api/settings/{key}                          remove the override
GET    /api/settings/system                         read-only runtime info: port, paths, log level, version, cipher_available
GET    /api/settings/ai                             read current AI config: {enabled, provider, model}
GET    /api/settings/ai/models                      provider model list
```

Keys live in the registry in `domain/settings/specs.go`; `backend/CLAUDE.md` lists them. AI provider, model and API key resolve DB → `PLANTRY_AI_*` env var → default at request time, so no restart is needed.

#### Availability and setup

```
GET    /api/availability?from=&to=                  resolved cells (plan / routine / none) and their source
GET    /api/availability/defaults                   weekday × slot defaults
PUT    /api/availability/defaults                   replace the defaults
PUT    /api/availability/{date}/{slotId}            override one cell (none on a cell with a plate: 409 unless remove_plate)
DELETE /api/availability/{date}/{slotId}            clear the override

GET    /api/setup                                   {completed, recipe_count, recipe_target}
POST   /api/setup/complete                          mark setup done
```

#### AI

```
POST   /api/ai/chat                                 SSE stream (rate-limited, default 10 req/min per IP)
                                                    body: {conversation_id?, message, today?, from, to}
GET    /api/ai/conversations                        list (limit, offset)
GET    /api/ai/conversations/{id}                   full history
DELETE /api/ai/conversations/{id}                   delete
GET    /api/ai/debug/system-prompt?from&to&today    only with PLANTRY_DEV_MODE
```

#### Import

```
POST   /api/import/extract                          body: {url} → draft recipe JSON
POST   /api/import/resolve                          body: {draft} → resolved recipe ready to save
GET    /api/import/lookup                           resolve one ingredient line
```

#### Images (static)

```
GET    /images/*                                    filesystem-backed image store
```

### 6.3 Error key catalogue

Error keys live in `frontend/src/lib/locales/{en,de}.json` under the `error.` namespace. Sample:

```
error.invalid_body
error.invalid_id
error.not_found
error.server
error.food.invalid_shop_section
error.food.shop_section_leaf_only
error.plan.invalid_start
error.plan.cell_occupied
error.plate.cell_occupied
error.plate.cell_unavailable
error.availability.cell_has_plate
error.feedback.invalid_status
error.import.fetch_failed
error.import.no_recipe
error.import.llm_failed
error.ai.provider_missing
error.ai.rate_limit_exceeded
error.profile.invalid_macros
```

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
| `/setup`                     | First-run setup wizard                               |
| `/settings/$section`         | Household, meals, cooking, assistant, system         |

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

Every multi-row mutation runs in one transaction through the unit of work (`domain.Transactor.InTx`, implemented by `sqlite.TxRunner`):

- Create or update a recipe (food + components + instructions + tags)
- Every `plate.Service` mutation: plates, components, cook sessions and their events
- Apply or copy a preset (`plate.Service.Apply`)
- Delete cascade (FK handles the rest)

Repositories take no transaction argument: inside `InTx` the transaction travels in the ctx and every repository call joins it. A nested `InTx` joins the outer one (no savepoints).

### 9.3 Logging

`log/slog` JSON handler. Request middleware logs method, path, status, duration, and a request ID. Error logs include stack info for unexpected errors only.

### 9.4 Auth

None. Plantry is a single-user app on a trusted LAN; there is no login, session or password gate. Put it behind a reverse proxy with auth if it must be reachable from outside the LAN.

### 9.5 Rate limiting

Token bucket on the LLM chat endpoint (default 10 req/min per IP) to prevent runaway costs. Configurable via env.

### 9.6 Observability

- `/api/health` returns `{status, version, db_ok}`.
- Request logs via slog.
- AI run log: every LLM call is recorded in `ai_runs` / `ai_run_steps` (kind, status, model, prompt version, tokens, per-call steps). Schema and example queries: `backend/CLAUDE.md` ("AI run log").
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
