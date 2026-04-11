# Plantry — Implementation Plan

This plan builds Plantry from an empty repository to production in **14 phases**. Each phase is a **vertical slice**: it touches the database, domain, adapters, transport, frontend, and e2e tests, and it ships a feature a user can actually use. Each phase follows **TDD**: tests are written before implementation, red first, green second, refactor third.

No phase builds on unfinished work. A phase is done only when:

1. All domain tests pass.
2. All adapter tests pass.
3. All handler tests pass.
4. All frontend unit tests pass.
5. At least one new Playwright e2e test covers the new feature and passes.
6. `bun run check && bun run lint` passes with zero warnings.
7. `go build ./... && go test ./...` passes.
8. The Docker image builds and boots.
9. A user can manually complete the phase's acceptance scenario end-to-end.

If any of these fail, the phase is not shipped. Period.

---

## TDD Discipline

Every phase cycles through the same inner loop for each new capability:

1. **Write the domain test first.** Use table-driven tests for pure functions; use service tests with fake repositories for services. Red.
2. **Write the minimum domain code to make it green.** No extra features.
3. **Write the adapter test.** Use `testhelper.NewTestDB(t)` for SQLite adapters; use a fake HTTP upstream for LLM/OFF/FDC adapters. Red.
4. **Implement the adapter.** Green.
5. **Write the handler test.** Use `httptest.NewRecorder`. Assert status code, response shape, and `message_key`. Red.
6. **Wire the handler.** Green.
7. **Write the frontend unit test (if the feature has pure logic).** Red.
8. **Implement the frontend code.** Green.
9. **Write the Playwright e2e test.** Red.
10. **Wire the UI until green.**
11. **Refactor** with the safety net of all tests passing.

Never skip steps. Never write "implementation first, tests later." If a test is hard to write, the design is wrong — change the design, not the test discipline.

---

## Phase 0 — Foundation (no user feature, but shippable)

**Goal:** a running empty shell that boots, serves a healthcheck, and passes CI.

**Deliverables:**

- `backend/cmd/plantry/main.go` with chi router, slog, graceful shutdown, `/api/health`
- `backend/internal/testhelper/db.go` — NewTestDB using in-memory SQLite + goose
- `backend/db/migrations/00001_init.sql` — empty baseline (just enables FKs, WAL pragma metadata)
- `frontend/` Vite 6 + React 19 + TypeScript scaffold with: TanStack Router (file-based), TanStack Query provider with `QueryClient`, shadcn/ui init, Tailwind v4, Lucide React, `react-i18next` wired but empty, `react-hook-form` + `zod`, Vitest + React Testing Library + Playwright configured
- Backend adds `sqlc` to toolchain: `sqlc.yaml` config, `internal/adapters/sqlite/queries/` directory, empty starter `.sql` file, `go generate` target that runs `sqlc generate`
- `Masterminds/squirrel` added as a dependency (used only where sqlc can't express dynamic filters)
- Tailwind theme: OKLch tokens from ARCHITECTURE.md section 2
- Dockerfile multi-stage (Bun builds Vite static output → Go compiles backend with `//go:embed` of the SPA → Alpine runtime)
- `docker-compose.yml` with named volume
- GitHub Actions CI: lint + typecheck + unit + e2e + docker build
- `CLAUDE.md` updated with redesign conventions

**Tests:**

- `handlers_test.go`: GET /api/health returns 200 + `{status:"ok"}`
- `sqlc generate` runs clean on an empty queries file
- Frontend Vitest smoke test renders the app with `QueryClientProvider` + `RouterProvider` and asserts the root nav renders
- Playwright smoke: app loads, renders "Plantry" in nav, no console errors

**Acceptance scenario:** `docker-compose up` boots, `curl /api/health` returns 200, frontend loads at `/` and shows an empty layout with botanical theme.

**Why this ships:** the foundation is a real deliverable. A broken foundation found later costs 10× more than one found now.

---

## Phase 1 — Ingredient Catalogue (manual)

**Goal:** users can manually create, edit, search, and delete ingredients with essential macros and optional image.

**Slice:**

- Migration `00002_ingredients.sql`: `ingredients` table + `ingredients_fts` + triggers (StatementBegin/End)
- `queries/ingredients.sql` for sqlc: `CreateIngredient`, `GetIngredient`, `UpdateIngredient`, `DeleteIngredient`, `ListIngredients`, `SearchIngredients`
- Domain `ingredient/`: aggregate, `Repository` port, `Service` with `Create`, `Update`, `Delete`, `Get`, `List`
- Adapter `adapters/sqlite/ingredient_repo.go` implementing the domain port via sqlc-generated queries (FTS via a raw query wrapped in the adapter)
- Handlers: `GET/POST /api/ingredients`, `GET/PUT/DELETE /api/ingredients/{id}`
- Error map entries: `error.ingredient.duplicate_name`, `error.ingredient.in_use`, `error.not_found`, `error.invalid_body`
- Frontend routes: `/ingredients`, `/ingredients/new`, `/ingredients/$id/edit`
- `lib/api/ingredients.ts` typed HTTP client
- `lib/queries/ingredients.ts` TanStack Query hooks: `useIngredients`, `useIngredient`, `useCreateIngredient`, `useUpdateIngredient`, `useDeleteIngredient`
- Components: `IngredientEditor` (react-hook-form + zod schema), `IngredientList`, `MacroFieldSet` (6 fields only)

**Tests (TDD order):**

1. Domain: `ingredient/service_test.go` — duplicate name returns ErrDuplicateName, create assigns ID, list respects search
2. Adapter: `ingredient_repo_test.go` — round-trip CRUD, FTS search finds by substring, constraint on duplicate name
3. Handler: `handlers/ingredients_test.go` — 201 on create, 409 on dup, 404 on missing, search works
4. Frontend unit: none critical in this phase
5. e2e: `ingredients.spec.ts` — create three ingredients, search one, edit one, delete one

**Acceptance:** a user can open `/ingredients`, create "Chicken breast" with kcal 165 / protein 31 / fat 3.6 / carbs 0, see it in the list, search for it, edit its values, and delete it.

---

## Phase 2 — Smart Ingredient Resolution

**Goal:** users rarely type macros. Barcode scan or name search populates everything.

**Slice:**

- Adapters `adapters/off/client.go` and `adapters/fdc/client.go` with HTTP fake upstream tests
- Adapter `adapters/imagestore/fs.go`: filesystem image storage + URL download, resize to max 1200 px long edge via `disintegration/imaging`, JPEG quality 85 encode, deterministic filenames
- Domain `ingredient/resolver.go`: takes a query (barcode OR name), calls OFF+FDC+local, returns ranked candidates
- Handlers: `GET /api/ingredients/lookup`, `POST /api/ingredients/resolve`, `POST/DELETE /api/ingredients/{id}/image`
- Ingredient portions: migration `00003_ingredient_portions.sql`, `queries/portions.sql` for sqlc, endpoints `GET/POST /api/ingredients/{id}/portions`, `DELETE /.../portions/{unit}`
- Frontend: `LookupPanel` component (uses `useLookup` query hook), `BarcodeScannerModal` using native `BarcodeDetector` API with lazy-loaded `@zxing/library` fallback for Firefox/Safari, `ImageUpload` with crop, `PortionsEditor`
- Ingredient editor uses lookup-first flow: search or scan, show candidates, one click to populate
- Background: the lookup panel is the primary creation path; the empty form is the escape hatch

**Tests:**

1. Adapter: OFF and FDC clients tested against recorded JSON fixtures (fake HTTP upstream)
2. Domain: resolver ranks local > FDC > OFF by default, deduplicates, tests for empty results
3. Handler: lookup endpoint proxies correctly, respects missing API keys, maps provider errors
4. Image store: writes to a temp dir, resizes to target dimensions, returns stable URL, cleans up on delete
5. Frontend unit: `LookupPanel` renders candidates, auto-selects first, supports manual override (React Testing Library)
6. e2e: `ingredient-resolution.spec.ts` — mock OFF/FDC responses via Playwright route intercept, create an ingredient by barcode in under five clicks

**Acceptance:** user clicks "Scan barcode", simulates a scan (or types it), sees a candidate auto-selected with image and macros, clicks save, ingredient exists.

---

## Phase 3 — Components (simple)

**Goal:** users can build a library of dishes with ingredients, instructions, role, image.

**Slice:**

- Migration `00004_components.sql`: `variant_groups`, `components`, `component_ingredients`, `component_instructions`, `component_tags`, `components_fts`
- `queries/components.sql` for sqlc (CRUD + children + FTS search)
- Domain `component/`: aggregate, ports, service with create/update/delete/get/list (role filter, tag filter, search)
- Domain `nutrition/calculator.go`: **the single source of truth**, pure functions, exhaustively tested
- Shared fixture file `testdata/nutrition-cases.json` — read by both Go (`nutrition/calculator_test.go`) and TS (`lib/domain/nutrition.test.ts`)
- `ingredient.Repository.LookupForNutrition(ids []int64)` helper
- Adapters: component SQLite repo using sqlc-generated queries with transactional child writes
- Handlers: full component CRUD + image + `GET /api/components/{id}/nutrition`
- Frontend routes: `/components`, `/components/new`, `/components/$id`, `/components/$id/edit`
- `lib/queries/components.ts`: `useComponents`, `useComponent`, `useCreateComponent`, `useUpdateComponent`, `useDeleteComponent`, `useComponentNutrition`
- Components: `ComponentEditor` (react-hook-form + zod, role selector, ingredient rows with portion-aware unit → grams resolution, instructions, tags, image, live nutrition preview via `useMemo` over pure `nutrition.ts`), `ComponentList` (grid/list, role chips, search, powered by `useComponents`)
- Frontend nutrition mirror `lib/domain/nutrition.ts` (pure TS) with Vitest table tests reading the shared fixture — CI fails on any divergence from Go

**Tests:**

1. Nutrition calculator Go tests: a matrix of ingredient combos → expected macros, including zero, null, large numbers, single ingredient, all six macros
2. Component service: create resolves unit → grams via ingredient portions, update replaces children transactionally, delete blocks if plates reference it (deferred to phase 5 but write the test with a stub)
3. Component repo: round-trip with children, FTS search on name
4. Handlers: happy path + invalid role (422) + duplicate child handling
5. Frontend nutrition Vitest: shares a JSON fixture file with Go tests; both implementations must return identical numbers
6. Frontend unit: `ComponentEditor` adds/removes ingredient rows, shows live nutrition preview
7. e2e: `components.spec.ts` — create a main component with four ingredients + three instructions + image, verify nutrition preview matches API response

**Acceptance:** user creates "Chicken curry" as role=`main` with four ingredients and three steps, sees its per-portion nutrition, edits it, can find it via search.

---

## Phase 4 — Variant Components

**Goal:** users can group variants of a dish (chicken curry / tofu curry / chickpea curry) for discoverability without duplicate data.

**Slice:**

- Add `variant_group_id` usage to component service (migration already exists from phase 3)
- Endpoints: `POST /api/components/{id}/variant` (clones skeleton into same group), `GET /api/components/{id}/variants`
- Service logic: creating the first variant auto-creates a variant group named after the parent; adding a second variant joins the group
- Frontend: "Create variant of…" button on component detail, "Other variants" section showing siblings as clickable cards
- Component picker (future phase 5) shows variant siblings as a stacked affordance

**Tests:**

1. Service: `CreateVariant(componentID)` creates a new component referencing the same group, first variant auto-creates the group, variants list returns siblings not self
2. Handler: endpoints return correct siblings, 404 on unknown, 409 if already in group
3. e2e: `variants.spec.ts` — create chicken curry, create tofu curry as variant, from tofu curry navigate to chicken curry via "Other variants"

**Acceptance:** user creates two components in a variant group and navigates freely between them.

---

## Phase 5 — Weekly Planner and Plates

**Goal:** users can plan meals by composing plates onto a weekly grid. **This is the heart of the product.**

**Slice:**

- Migration `00005_planner.sql`: `weeks`, `time_slots`, `plates`, `plate_components`
- `queries/planner.sql` for sqlc (weeks, plates, plate_components CRUD + aggregated week read)
- Domain `planner/`: week service (get/create current, by-date, copy), plate service (create, update, delete, add-component, swap-component, update-component, remove-component)
- **Critical test:** swap-component must update the component ID on an existing plate_component row and preserve sort_order; removing a component must not orphan the plate
- Copy-week service: deep-clones plates + plate_components transactionally
- Handlers: full week + plate + plate_components endpoints per ARCHITECTURE.md §6.2
- Settings endpoints for time slots: `GET/POST /api/settings/slots`, `PUT/DELETE /api/settings/slots/{id}`
- First-run flow: if no time slots exist, the planner renders an empty state pointing to Settings
- Frontend routes: `/` (planner), `/settings` (time slots only for now)
- `lib/queries/weeks.ts` + `lib/queries/plates.ts`: TanStack Query hooks for current week + all plate mutations, each implementing the canonical optimistic update pattern from ARCHITECTURE.md §8.3 (`onMutate` snapshot → patch → `onError` rollback → `onSettled` invalidate week + shopping-list + nutrition)
- `lib/stores/planner-ui.ts`: Zustand store for ephemeral UI only (currently-edited plate, panel visibility, drag state)
- Components: `PlannerGrid` (desktop), `PlannerDayView` (mobile), `PlateCell`, `PlateEditor` (modal/sheet with role-aware component picker), `PlateComponentChip` (with swap affordance), `TimeSlotsEditor`
- Drag-and-drop: `dnd-kit` for moving plates between day/slot cells and reordering components within a plate — keyboard + touch sensors enabled
- Role-aware picker: "Pick a main" filter auto-applied; user can override filter

**Tests:**

1. Domain: plate service test matrix — create plate in a slot that has no plate, swap a component, remove a component, add a second main (allowed — we don't forbid it), delete cascades plate_components
2. Copy-week test: three plates copied into a target week, children preserved, original untouched
3. Adapter: plate repo round-trip including sort_order preservation
4. Handler: 404 on unknown plate, 409 on slot without a time slot defined, role filter works in picker
5. Frontend hooks: mutation rollback tests — mock `fetch` to fail, assert `useSwapComponent` restores previous week data in the query cache (React Testing Library + `renderHook`)
6. e2e: `planner.spec.ts` — define three time slots, plan Monday dinner with chicken curry + basmati + raita, swap basmati for naan (via dnd-kit keyboard path for a11y coverage), remove raita, navigate to next week and back

**Acceptance:** user defines slots, opens planner, creates a plate with three components, swaps one, removes one, navigates weeks, copies a week forward.

---

## Phase 6 — Shopping List and Nutrition Views

**Goal:** users see what to buy and how their week looks against macros.

**Slice:**

- Domain `shopping/aggregator.go`: pure function `FromPlates([]Plate) []ShoppingItem`, sums grams per ingredient across the week
- Domain `nutrition.WeekTotals(week)` using existing calculator
- Handlers: `GET /api/weeks/{id}/shopping-list`, `GET /api/weeks/{id}/nutrition`
- Frontend: `ShoppingPanel` (slide-over), `NutritionDayBar`, `NutritionWeekSummary` (without targets yet — targets land in phase 8)
- Click-through-purchased state is client-side localStorage only, per-week key

**Tests:**

1. Pure function tests for the aggregator (tables of plate compositions → expected grams)
2. Nutrition week totals: day-sum and week-sum correctness
3. Handler: both endpoints
4. e2e: `shopping-and-nutrition.spec.ts` — plan a week with known components, open shopping list, assert totals match hand-calculated values; see day bars render

**Acceptance:** user views shopping list aggregated across the planned week, views nutrition bars per day.

---

## Phase 7 — Templates (saved combos)

**Goal:** users save frequent plate compositions and apply them with one click.

**Slice:**

- Migration `00006_templates.sql`: `templates`, `template_components`
- Domain `template/service.go`: create (optionally from existing plate), apply to plate (replaces contents or merges — decision: **replace by default**, merge via explicit flag)
- Handlers: template CRUD + apply endpoint
- Frontend routes: `/templates`, `/templates/new`
- "Save as template" action on plate editor
- "Apply template" action on empty plates

**Tests:**

1. Service: create-from-plate copies children, apply replaces plate contents, transactional
2. e2e: `templates.spec.ts` — build a plate, save as template "Curry night", create a new plate on another day, apply template, verify three components appear

**Acceptance:** user saves and re-uses a plate composition.

---

## Phase 8 — Profile and Nutrition Targets

**Goal:** users set goals and see progress.

**Slice:**

- Migration `00007_profile.sql`: `user_profile` singleton
- Domain `profile/service.go`: get/update, enforces macro % sum ≤ 100, validates kcal_target > 0 if present
- Handlers: `GET/PUT /api/profile`
- Frontend: Profile section in `/settings` with goal mode presets (cut / maintain / bulk) + custom fields, dietary restrictions (chip editor), system_prompt textarea, locale selector
- `NutritionDayBar` now shows delta vs targets (if set)
- `NutritionWeekSummary` shows average vs targets
- Empty-targets state is graceful — just shows absolute totals, no judgment

**Tests:**

1. Service: validation, idempotent update
2. Handler: 400 on invalid macro sum
3. Frontend: preset buttons populate fields, custom override works
4. e2e: `profile.spec.ts` — set targets via "cutting" preset, plan a day, bars reflect progress

**Acceptance:** user sets targets, plans meals, sees progress bars against goals.

---

## Phase 9 — AI Agent Foundation

**Goal:** users can chat with an AI that actually edits the plan.

**Slice:**

- Domain `llm/`: `Client` interface, `Tool` type, `Request`/`Response`
- Adapter `adapters/openai/client.go` and `adapters/anthropic/client.go`, tested against JSON fixtures of real API responses (no live calls in CI)
- Domain `planner/agent.go`: tool-calling loop (max 20 iterations), system prompt composer
- Tools (each backed by existing domain services, no SQL):
  - `list_components`, `get_component`
  - `get_week`, `get_week_nutrition`, `get_profile`
  - `create_plate`, `add_component_to_plate`, `swap_component`, `update_plate_component`, `remove_plate_component`, `delete_plate`, `clear_week`
- Transport: `POST /api/ai/chat` SSE stream emitting `assistant`, `tool_call`, `tool_result`, `plate_changed`, `done`, `error`
- AI settings endpoints: `PUT /api/settings/{provider|model|api_key}`, `GET /api/settings/ai/models` (proxies provider model list)
- Migration `00008_ai.sql`: `ai_conversations`, `ai_messages`
- Frontend: `ChatPanel` (slide-over on planner), `ChatMessage`, `ToolCallBlock`, SSE reader in `lib/api/ai.ts`, `useChatStream` hook in `lib/queries/ai.ts` owning the reader lifetime
- `useChatStream` invalidates `qk.weeks.byId(weekId)`, `qk.weeks.nutrition(weekId)` and `qk.weeks.shoppingList(weekId)` on every `plate_changed` event — TanStack Query refetches automatically
- Ephemeral chat UI state (draft message, panel open) lives in `lib/stores/chat-ui.ts` Zustand store; conversation history is a TanStack Query
- Chat history persisted per week via conversations table
- Mode selector: "fill empty slots" / "replace all"

**Tests:**

1. LLM adapters: wire format tests with recorded fixtures — request encoding, response parsing, tool_use blocks, error cases
2. Agent loop test with a **fake LLM** (deterministic scripted responses): scenario "user says plan Tuesday dinner" → agent calls list_components, get_component, create_plate, add_component_to_plate — verify all calls happened in order, verify plate exists in DB
3. SSE test: handler emits events in correct order, closes on done
4. Frontend unit: SSE buffer parser handles split chunks, interleaved events
5. e2e: `ai-chat.spec.ts` — intercept /api/ai/chat and replay a canned SSE stream, verify UI shows assistant message, tool blocks, and that the planner grid updates to reflect the "created" plate

**Acceptance:** with a mocked backend stream, a user asks the agent to plan Tuesday dinner and sees the plan appear in the grid in real time.

---

## Phase 10 — AI Memory and Feedback Loop

**Goal:** the AI learns from what the user actually cooks and enjoys.

**Slice:**

- Migration `00009_plate_feedback.sql`: `plate_feedback` table
- Domain `feedback/service.go`: record feedback, update `components.last_cooked_at` + `cook_count` on `cooked`/`loved`
- Domain `profile/preferences.go`: heuristic updater — on `loved`, append component tags to `preferences.likes`; on `disliked`, append to `preferences.dislikes`. Pure function, unit-tested.
- Agent tool: `record_preference(key, value)` for explicit writes
- System prompt composer includes profile preferences
- Frontend: feedback buttons on plate cards (cooked ✓, skipped ✗, loved ❤, disliked ✗), note field
- Conversation history browser in `/settings` or chat panel
- Archive integration deferred to phase 12

**Tests:**

1. Feedback service: `cooked` increments cook_count and updates last_cooked_at
2. Preferences heuristic: loved curry tagged "spicy" → `preferences.likes.spicy = true`
3. Agent test: with seeded preferences, system prompt includes them
4. e2e: `feedback.spec.ts` — mark a plate as loved, open new chat, verify system prompt (via debug endpoint in dev mode) contains the preference

**Acceptance:** user cooks and marks a plate, the preference is recorded, a new AI conversation picks up on it.

---

## Phase 11 — Recipe Import from URL

**Goal:** users import recipes from the web into components.

**Slice:**

- Adapter `adapters/jsonld/parser.go`: schema.org Recipe extraction from HTML
- Domain `importer/service.go`:
  - Step 1: fetch URL (30 s timeout, 5 MB limit, UA string)
  - Step 2: try JSON-LD; fallback to LLM via `llm.Client` with a strict extraction prompt
  - Step 3: parse ingredient strings (amount, unit, name) via regex helper
  - Step 4: resolve each ingredient via `ingredient/resolver.go` from phase 2
- Handlers: `POST /api/import/extract` (returns draft), `POST /api/import/resolve` (applies resolution + returns component-shaped JSON)
- Frontend route: `/import`, wizard: paste URL → review draft → resolve ingredient rows (auto-selected by default, manual override available) → assign role → save

**Tests:**

1. JSON-LD parser: fixtures from real schema.org recipes, including single-object and array shapes, ISO 8601 duration parsing
2. Importer service: with a fake LLM for fallback, with a fake HTTP fetcher, assert correct pipeline order and error surfaces
3. Ingredient string parser: table of input strings → (amount, unit, name)
4. e2e: `import.spec.ts` — intercept network, serve a stub recipe HTML, run wizard, save, verify component appears in library

**Acceptance:** user pastes a recipe URL, reviews the draft, confirms ingredient matches, saves — a new component exists.

---

## Phase 12 — Archive and Rotation Insights

**Goal:** users browse history and avoid cooking the same things every week.

**Slice:**

- Handlers: `GET /api/weeks` paginated; component list sort order `last_cooked` already supported
- Domain `component/service.go`: `InsightsQuery` returns "not cooked in N weeks" and "top N most cooked"
- Handler: `GET /api/components/insights`
- Frontend route: `/archive`, `/archive/[id]`
- Component library: "Forgotten" and "Most cooked" badges on cards
- Archive uses the read-only planner grid component

**Tests:**

1. Insights query: DB seeded with known cook counts and dates, assert correct buckets
2. e2e: `archive.spec.ts` — plan past weeks (via direct DB seeding in the test), view archive, see insights on component library

**Acceptance:** user sees past weeks and rotation insights on the component library.

---

## Phase 13 — Production Polish

**Goal:** ship-ready.

**Slice:**

- PWA via `vite-plugin-pwa` (auto-generated manifest + service worker, cache shell + static assets, auto-update strategy)
- German translation file (parity with en.json; translation quality is a stretch, structural parity is the bar)
- Print stylesheet for the planner grid (weekly meal card) and shopping list
- Endpoint `GET /api/weeks/{id}/calendar.ics` (iCal export) with each plate as an event at its slot's default time
- Optional single-password auth middleware gated by `PLANTRY_AUTH_PASSWORD`, login page at `/login`
- Rate limit middleware on `/api/ai/chat` (token bucket, 10/min per IP, env-configurable)
- Image optimization: resize uploads to max 1200 px on the long edge, convert to JPEG with quality 85
- Final Docker image size audit (target < 35 MB)
- Full Playwright smoke pass on the whole app
- Backup/restore instructions in README

**Tests:**

1. Auth middleware: unauth 401, valid password 200, cookie flow works
2. iCal handler: valid ICS output parseable by a known parser library (test-only dep)
3. Rate limiter: burst + sustained test
4. Image resizer: input > 1200 px becomes ≤ 1200 px, aspect ratio preserved
5. e2e: `polish.spec.ts` — PWA manifest served, iCal endpoint returns valid content-type, login flow when password set

**Acceptance:** a fresh deployment on a Raspberry Pi runs, serves the PWA, supports login, exports iCal, prints the weekly view, and survives a sustained 100-request burst to the chat endpoint.

---

## Phase summary table

| Phase | Feature shipped                                 | Key risks                                    |
| ----- | ----------------------------------------------- | -------------------------------------------- |
| 0     | Empty shell boots, CI green                     | Toolchain setup, Docker size                 |
| 1     | Manual ingredient CRUD                          | FTS trigger goose formatting                 |
| 2     | Barcode + FDC lookup, images, portions          | BarcodeDetector support, provider API quirks |
| 3     | Component library with nutrition                | Calculator parity Go ↔ TS                    |
| 4     | Variant groups                                  | Low — small slice                            |
| 5     | Weekly planner + plates (core feature)          | Optimistic state reconciliation              |
| 6     | Shopping list + nutrition views                 | Aggregation correctness                      |
| 7     | Templates                                       | Low                                          |
| 8     | Profile + goal targets                          | Low                                          |
| 9     | AI agent with tool calls + SSE                  | LLM fixture stability, loop termination      |
| 10    | Feedback + learned preferences                  | Heuristic quality (acceptably naive)         |
| 11    | URL import                                      | Scraping resilience, LLM fallback cost       |
| 12    | Archive + rotation insights                     | Low                                          |
| 13    | PWA, i18n, print, iCal, auth, rate limit        | Auth UX, PWA cache invalidation              |

Each phase is self-contained and shippable. Stopping after any phase leaves a coherent product — slightly less capable, but not broken.

---

## Cross-phase engineering rules

1. **No phase is allowed to bypass tests.** If something is hard to test, refactor the design.
2. **No phase duplicates nutrition math.** Every macro number comes from `nutrition/calculator.go` or its TS mirror.
3. **No phase touches the database from a handler.** Handlers call services. Services call repositories.
4. **No phase introduces a new cross-cutting concern without updating ARCHITECTURE.md.** Docs are part of the deliverable, not an afterthought.
5. **No phase merges with failing e2e.** If Playwright is flaky, fix Playwright, don't `retry(3)`.
6. **Rollback plan for every migration:** every goose migration has a `-- +goose Down` section that is tested by at least one test running up, down, up.
7. **Performance budget:** any API endpoint that returns more than a handful of rows must paginate. Any page must render in under 200 ms on a Raspberry Pi 3 B+ with a typical dataset (50 components, 10 weeks planned).
8. **Every new error key is added to en.json, de.json, AND generated Go constants in the same commit.**
9. **Every new environment variable is documented in README and config validator.**
10. **Commit discipline:** one logical change per commit, descriptive subject (imperative), body explains "why", never mix formatting with logic changes.
11. **Server state lives in TanStack Query; client state in Zustand.** A new `useState` holding fetched data is a bug — replace with a query hook. A new Zustand store mirroring server data is a bug — delete it and use a query.
12. **Every repository adapter uses sqlc-generated queries.** Hand-written scan code is only allowed for the 3–4 places where sqlc's static analysis can't express the query (dynamic optional filters) — those use `squirrel`.

---

## What ships at the end of Phase 13

A self-hosted meal planning web application that:

- Runs in a single ~30 MB Docker container on a Raspberry Pi.
- Lets users build a component library (dishes by role), compose plates onto a weekly grid, swap components in one click, save common combinations as templates.
- Auto-fills ingredient nutrition from barcode scans, FDC search, and Open Food Facts, with image upload and portion units.
- Imports recipes from URLs via JSON-LD with LLM fallback.
- Tracks macros against user targets with goal presets.
- Chats with an AI agent that reads the profile, uses the library, edits plates in real time via tool calls, and learns from cooked/loved/disliked feedback.
- Archives past weeks and surfaces rotation insights.
- Supports English and German, prints a weekly card, exports iCal, and optionally gates behind a household password.
- Has a vertical test suite (domain unit → adapter → handler → e2e) with no untested features.

That is Plantry v1.
