# Phase 0 implementation plan: Foundations

Status: ready to execute. Written 2026-10-26 against `main` at e0a9e68.

Sources of truth:
- `docs/PLANNING_ASSISTANT_SPEC.md`: §14 Phase 0, plus §4.4, §8.1 and §10.
- `docs/UX_DESIGN_SPEC.md`: §2.1, §3 and §9 (Phase 0 UI). The mockup is `docs/design/visual-directions.html`, section "Chosen".

All file and line references below were checked against the code on 2026-10-25/26. Where the spec's audit references (from 2026-10-20) are out of date, this plan gives the current location.

---

## 0. Definition of done

1. The existing chat produces correctly dated plans: the system prompt names today, the window dates and their weekdays.
2. Importing a chefkoch.de recipe gives correct macros with no manual ingredient fixing.
3. Every AI call has a row in `ai_runs` with model, prompt version, tokens and status.
4. The app uses the new tokens and fonts, and has four-destination navigation.
5. These all pass:
   - `cd backend && go test -race ./... && go vet ./... && golangci-lint run`
   - `cd frontend && bun run check && bun run e2e`

---

## 1. Decisions taken for Phase 0

The user may override any of these.

| # | Decision | Reason |
|---|---|---|
| D1 | **Nav targets.** Today → `/day/<today>`. Plan → `/`. Recipes → `/components`, with Ingredients and Import linked from its header. Shop → a new thin `/shop` page. Route paths stay unchanged; the renames (`/recipes`, `/today`, `/plan`) happen with the rebuilt screens in Phases 1–3. On phones the Settings gear sits in the global TopBar in Phase 0, because the Today header it belongs in (UX §2.1) arrives with the Phase 2 Today screen. | Four destinations now, without building Phase 2 screens early. |
| D1a | **Nav restructure is Phase 0.** UX §9 places it in Phase 0, which overrides planning spec §14 listing "Nav fixes" under Phase 1. | The UX spec is the newer, more specific source. |
| D2 | **Tokens by aliasing.** The new §3.1 tokens are the source. Legacy Material utilities (`bg-surface-container-*`, `text-on-surface*`, `border-outline*`, `*-ai-accent*`) and shadcn variables are remapped onto them in `index.css`. The ~800 class usages are not renamed in Phase 0; each screen moves to the new names when it is rebuilt. | The whole app changes in one step and the diff stays reviewable. |
| D3 | **Run-log scope.** Phase 0 adds only the tables, recording and logs. The cost table (`llm.Cost`), `ai.price_overrides`, any runs API and the insights page move to Phase 3. In Phase 0, `ai_runs` is verified by tests and by `sqlite3`. | DoD 3 only needs the rows. |
| D4 | **Extra run kinds.** `lookup` covers food-lookup translate and pick-best. `untracked` is a safety net for a metered call made without a started run. `running` is the in-progress status. | §8.1 has no kind for food lookup, and a forgotten call site should still be visible. |
| D5 | **Metering at the client factory.** The metered client wraps each concrete client inside the `llmFactory` closure in `main.go`, which knows the provider. The model comes from `Request.Model`. `llm.Resolver` is unchanged. | Every caller is covered with no adapter or resolver changes. |
| D6 | **Deferred to Phase 1:** full per-food count portions (§10.4 first half) and diet-class classification on import (§10.6). Phase 0 fixes the parser bugs that produce wrong grams. | The parser already gives fixed gram defaults for count units, so nothing crashes today. |
| D7 | **Recipe detail, Phase 0 subset.** A reading view at `/components/$id` shows photo, name, source link, kcal and protein per portion, portions, ingredients, steps, My notes, the imported description (muted) and the completeness warning. Actions: Edit and Favorite. Cook now, the portions stepper and cook history come in Phase 2. | UX §9 Phase 0 lists "recipe detail/import fixes". |
| D8 | **Chat dates come from the client.** The chat request carries `today`, `from` and `to` as `YYYY-MM-DD` from the browser. The server validates them and uses the server clock only when `today` is missing. | The container often runs in UTC; the browser knows the user's day. |
| D9 | **Impeccable without the helper binary.** Never run `.claude/skills/impeccable/scripts/impeccable` without asking the user first. Use the documented fallback: read `docs/PRODUCT.md`, `DESIGN.md` (if present) and the skill's `reference/*.md` directly. Ask the user before the `$impeccable document` pass that writes `DESIGN.md`. | User instruction. |

---

## 2. Repository facts the steps rely on

- **Migrations** are in `backend/db/migrations`; the latest is `00020_drop_templates.sql`.
  - The next numbers are fixed: **00021 = A1**, **00022 = C4**.
  - sqlc reads `db/migrations` and `db/schema_patches`. Queries are in `backend/internal/adapters/sqlite/queries/*.sql` and generated code in `internal/adapters/sqlite/sqlcgen`.
  - **Generated-file conflicts:** when two branches both ran `sqlc generate`, resolve by taking either side and re-running `sqlc generate` on the merged tree. Never hand-merge `sqlcgen/`.
- **`llm.Client`**: `Stream(ctx, req, out)` returns `(*Response, error)` with `Usage`. `Complete(ctx, req)` returns only `(string, error)` and carries no usage.
  - `llm.Resolver.Current(ctx)` returns `(Client, model, error)`.
  - The factory closure is at `backend/cmd/plantry/main.go:146`; `llmresolver.New` is at `:157`.
- **LLM call sites:**
  - `agent/loop.go:68` (Stream)
  - `food/ai.go:80` translate and `:144` pick-best (Complete)
  - `importer/llm_prompt.go:49` and `:68` (Complete, one retry)
  - `handlers/ai.go:42` only calls `Current`.
- **Agent:**
  - `service.go:186` (`buildSystemPrompt`) calls `ComposePrompt(p, nil, nil, prefs)`, so no dates reach the prompt.
  - `service.go:202` (`DebugSystemPrompt`) passes `nil, nil, nil`.
  - `prompt.go` already has `DateRange` and `writeDateRangeSection`, but production never uses them.
  - `loop.go`: `DonePayload.Usage` holds only the last iteration's usage.
- **Agent tools:** `foodSummary` (`tools.go:598-615`) and `plateSummary` (`tools.go:678-687`) lack favorite, cook data, macros and skipped.
- **Nutrition:** the silent zero is `deref` at `food/nutrition.go:162-165`, used by `leafPer100g` (`:129`). The spec's `:236` is stale.
  - `PerPortion` callers: `handlers/plate_macros.go`, `handlers/nutrition.go:100`, `handlers/foods.go:499` (GET `/foods/{id}/nutrition`) and `:560` (BatchMacros), and the agent tools.
- **Resolver:**
  - `food/resolver.go` `lookupQuery` searches FDC only and returns nothing without an FDC key.
  - `enrichExistingIDs` compares local foods against the FDC description, because it runs before `results[i].Name = originalQuery`.
  - The OFF provider has `SearchByName`.
  - `handlers/lookup.go:184-185` refetch passes the FDC id as a text query. `adapters/fdc/client.go:104` already has `GetFood(ctx, fdcID)`; `DynamicProvider` does not expose it.
- **Import:**
  - `adapters/jsonld/parser.go:118` reads `recipeYield` with `stringOf`, which returns `""` for numbers and arrays.
  - Parser probes:
    - `"100g Mehl"` comes out unparsed.
    - `"2 Zehe/n Knoblauch"` becomes 240 g named "Zehe/n Knoblauch".
    - `"1 Zwiebel(n)"` gets the note "n".
  - Frontend `routes/import/index.tsx`:
    - `:102` sends `notes: draft.description`.
    - `:116-130` creates without the source URL or image.
    - `:466-475` disables candidates without `existing_id`.
    - `onCreateManual` (~`:380`) swallows errors and fails on the unique leaf name `ux_foods_name_leaf`.
  - `frontend/e2e/import.spec.ts` posts fixture HTML to `/api/import/extract` with `{html}`. Reuse that pattern; no network.
- **Images:** `adapters/imagestore` has `SaveFromURL(ctx, url, category, id)`.
- **Config:** `PLANTRY_SECRET_KEY` is read at `config/config.go:95-96`; the cipher is chosen at `main.go:83-90`. `DBPath` defaults to `/data/plantry.db`, and there is no data-dir setting.
- **Frontend chat:** `ChatPanel.tsx` passes `range` into the mutation, but `lib/queries/ai.ts` only uses it to invalidate caches (`:151`). The request body sends `conversation_id`, `week_id` and `mode`.
- **Shopping:**
  - `ShoppingPanel.tsx:78-101` anchors its presets on `new Date()`. `routes/index.tsx:540` mounts it with a key.
  - The fallback shopping day disagrees: backend `specs.go` uses `"0"`, `routes/index.tsx:118` uses `"1"`, `components/settings/tabs/PlanTab.tsx:71` uses `"5"`.
- **Shell:**
  - `SideNav.tsx` has 6 items (/, /components, /ingredients, /presets, /calendar, /settings). `MobileBottomNav.tsx` has 5 and no settings.
  - `index.css` imports Inter and Manrope (`package.json:22-23`).
  - E2E specs that rely on nav: `shell`, `catalog-grid`, `archive-redirect`, `components`, `day-detail`, `planner-picker`, `inventory-grid`.
- **Baseline (2026-10-25):** `go test ./...` passes; `bun run check` passes (51 files, 379 tests).

---

## 3. Steps

Every step:
- Load the required skills first: Go uses `golang-patterns` and `golang-testing`; frontend uses `frontend-design`, `vercel-react-best-practices`, `vercel-composition-patterns`, `shadcn`, `vitest` and `playwright-best-practices`; UI work adds `impeccable` (see D9).
- Write the listed tests first and watch them fail, then implement, then run the step's verification.
- Changes are surgical (root `CLAUDE.md` §3). User-visible strings go through i18n with both `en.json` and `de.json`.

### Track A: AI run log

#### A1. Run-log tables, store port and SQLite repo

- **Files**
  - new `backend/db/migrations/00021_ai_runs.sql`
  - new `backend/internal/adapters/sqlite/queries/ai_runs.sql`
  - regenerated `sqlcgen/`
  - new `backend/internal/adapters/sqlite/ai_run_repo.go` and `_test.go`
  - new `backend/internal/adapters/sqlite/migration_00021_test.go`
  - new `backend/internal/domain/airun/store.go`: types plus the `Store` port only
- **Schema**, matching §8.1 plus D4:
  ```sql
  CREATE TABLE ai_prompts (hash TEXT PRIMARY KEY, text TEXT NOT NULL);
  CREATE TABLE ai_runs (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('generate','regenerate','chat','discovery','classify','import_fallback','lookup','untracked')),
    started_at TEXT NOT NULL, finished_at TEXT,
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','ok','error','iterations_exceeded','cancelled','partial')),
    provider TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '', prompt_hash TEXT REFERENCES ai_prompts(hash),
    window_start TEXT, window_end TEXT, scope_json TEXT, instruction TEXT,
    chooser TEXT CHECK (chooser IS NULL OR chooser IN ('llm','heuristic')),
    iterations INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    repair_attempts INTEGER NOT NULL DEFAULT 0,
    violations_json TEXT, score_json TEXT, judge_json TEXT,
    outcome TEXT CHECK (outcome IS NULL OR outcome IN ('accepted','discarded','partially_accepted','open')),
    outcome_at TEXT, error_key TEXT
  );
  CREATE INDEX ix_ai_runs_started ON ai_runs(started_at);
  CREATE TABLE ai_run_steps (
    id INTEGER PRIMARY KEY,
    run_id INTEGER NOT NULL REFERENCES ai_runs(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL, type TEXT NOT NULL CHECK (type IN ('llm_call','tool_call')),
    name TEXT NOT NULL, args_json TEXT, result_json TEXT, is_error INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
    UNIQUE (run_id, seq)
  );
  ```
  Timestamps use the same text format the other tables use; check `plate_repo.go` and copy it.
- **Port** (`domain/airun/store.go`):
  - `Store` has `CreateRun(ctx, RunStart) (int64, error)`, `FinishRun(ctx, RunEnd) error`, `AddStep(ctx, Step) error` and `UpsertPrompt(ctx, hash, text) error`.
  - `RunStart`, `RunEnd` and `Step` are plain structs. `RunEnd` carries status, finished_at, provider, model, prompt hash, iterations, all four token counts and error_key.
- **Tests first**
  - The migration creates all three tables; an invalid `kind` or `status` is rejected; deleting a run cascades to its steps.
  - Repo round-trip: create, two steps, finish, read back. Add a test-only read query or use raw SQL in the test.
  - `UpsertPrompt` with the same hash twice leaves one row.
  - `result_json` longer than 8192 bytes is stored truncated to 8192 bytes plus the marker `…[truncated]`. The repo truncates, and the cut lands on a valid UTF-8 boundary.
- **Verify:** `cd backend && sqlc generate && go test ./internal/adapters/sqlite/ -run 'AIRun|00021' && go vet ./...`

#### A2. Recorder and metered client

- **Files**: new `backend/internal/domain/airun/recorder.go`, `meter.go` and `_test.go`. Uses an in-memory `Store` fake in the test file.
- **Design**
  - `Recorder` is built with `NewRecorder(store Store, logger *slog.Logger, now func() time.Time)`.
  - `Start(ctx, Start{Kind, PromptVersion, WindowStart, WindowEnd, Instruction}) (context.Context, *Run)` creates the row, puts `*Run` in ctx and logs `ai.run.start` with run_id, request_id (chi `middleware.GetReqID`) and kind.
  - `Run.Finish(ctx, status, errKey)` writes totals and logs `ai.run.end` with run_id, request_id, kind, provider, model, iterations, tokens, status and duration_ms.
    - It is idempotent: a second call is a no-op.
    - It uses `context.WithoutCancel(ctx)` so a cancelled request still records `cancelled`.
  - `Run` holds a mutex. It sums usage, counts `llm_call` steps as `iterations`, assigns `seq`, and keeps the provider, model and prompt hash from the first call.
  - `Run.ToolStep(ctx, name, args, result, isErr, dur)` is the hook used by agent tools.
  - `FromContext(ctx) *Run` returns nil when the context has no run.
  - `(r *Recorder) Meter(inner llm.Client, provider string) llm.Client`:
    - **`Stream`:** record start time, call `inner.Stream`, then record an `llm_call` step with `name = req.Model`, tokens from `resp.Usage`, duration, and `is_error = err != nil`. Hash `req.System` (sha256 hex) and `UpsertPrompt` once per run.
    - **`Complete`:** implement it by calling `inner.Stream` with an internal sink and concatenating the text deltas, so tokens are recorded.
      - Before relying on this, read both real adapters (`adapters/anthropic`, `adapters/openai`) and the fake.
      - If any adapter's `Complete` differs materially from `Stream`, for example in response format or `max_tokens` handling, use `inner.Complete` for that call instead and record the step with zero tokens. Note the gap in the step's `result_json` as `{"usage":"unavailable"}`.
    - **No run in ctx:** start an implicit run with kind `untracked` and finish it after that single call. A forgotten call site is then visible rather than lost.
    - **Store failures:** log them at warn and never fail the AI call.
  - Constants: each prompt template owns a `PromptVersion` string, starting at `"v1"` and bumped on every change. Define them in their packages (agent, importer, food), not in airun.
- **Tests first** (table-driven, using the fake client from A3 and the fake store):
  - Three `Stream` calls in one run give summed input, output and cache tokens and `iterations = 3`.
  - `Complete` returns exactly the text the inner client would, and records usage.
  - An inner error gives a step with `is_error=1`, and the caller still receives the error unchanged.
  - A call outside a run gives one `untracked` run with status `ok`.
  - `Finish` twice writes once.
  - A cancelled ctx gives the `cancelled` status written.
  - Concurrent `ToolStep` calls are safe: run with `-race` and 50 goroutines, and every `seq` is unique.
  - The prompt is stored once across three calls with the same system text.
- **Verify:** `cd backend && go test -race ./internal/domain/airun/`

#### A3. Fake client records requests

- **Files**: `backend/internal/adapters/fake/client.go` and `_test.go`.
- **Change**: append every `llm.Request` received by `Stream` and `Complete` to a mutex-guarded slice, and add `Requests() []llm.Request`, which returns a copy. Extend `ScriptedUsage` with `cache_read_tokens` and `cache_write_tokens` (JSON, optional) so metering tests can check all four counters.
- **Tests first**: two calls give two recorded requests with the right `System` and `Model`; concurrent calls pass under `-race`; a script with cache tokens returns them in `Usage`.
- **Verify:** `cd backend && go test -race ./internal/adapters/fake/`

#### A4. Wire every call site

Needs A2 and A3.

- **Files**
  - `cmd/plantry/main.go`: build the recorder and wrap inside `llmFactory` (D5)
  - `internal/domain/agent/service.go` and `loop.go`
  - `internal/domain/agent/tools.go`: `ToolSet.Execute` hook
  - `internal/domain/food/ai.go` and `resolver.go`
  - `internal/domain/importer/llm_prompt.go` and `service.go`
  - their tests
- **Behaviour**
  - **Chat.** The agent service starts a `chat` run (instruction = user text; window from B1 once it lands) and finishes it with status:
    - `ok` on success
    - `iterations_exceeded` when `MaxIterations` is hit
    - `cancelled` on context cancel
    - otherwise `error`, with `error_key` set to the existing i18n error key
  - `ToolSet.Execute` calls `airun.FromContext(ctx).ToolStep(...)` (nil-safe).
  - `loop.go` sums usage across iterations into `DonePayload.Usage`, fixing the "last iteration only" bug.
  - **Import fallback.** `llmExtract` runs inside an `import_fallback` run, with both attempts as steps of one run.
  - **Food lookup.** A resolve that calls translate and/or pick-best runs inside one `lookup` run. No run is started when no LLM call happens.
  - Add `PromptVersion` constants in `agent`, `importer` and `food`.
- **Tests first**
  - Each call site has a test using the fake client and a real SQLite store (in-memory or temp file, as the existing repo tests do). Each asserts exactly one `ai_runs` row with the right kind, model, prompt_version, summed tokens and status, plus the expected steps (for chat, one `llm_call` per iteration and one `tool_call` per tool use).
  - An `iterations_exceeded` scenario records that status.
  - `DonePayload.Usage` equals the sum across iterations.
- **Verify:**
  ```bash
  cd backend && go test -race ./internal/domain/... ./cmd/... && golangci-lint run
  ```
  Then run the app with `PLANTRY_AI_PROVIDER=fake` (or whatever the e2e config uses; see `frontend/playwright.config.ts`), send one chat message, and check:
  ```bash
  sqlite3 <db> "select kind,provider,model,prompt_version,input_tokens,output_tokens,status from ai_runs order by id desc limit 5"
  ```

### Track B: Agent

#### B1. Dates in the chat prompt

Needs A3. Runs after A4 because both edit `agent/service.go`.

- **Files**
  - Backend:
    - `internal/transport/http/handlers/ai.go` and `_test.go`
    - `internal/domain/agent/service.go`, `prompt.go`, `prompt_test.go` and `service_test.go`
    - the main.go wiring if the service needs a plate reader and a clock
  - Frontend:
    - `src/lib/api/ai.ts`
    - `src/lib/queries/ai.ts` and its test
    - `ChatPanel.tsx` and any other chat caller, e.g. `GeneratePlanButton`; grep for the chat mutation
- **Behaviour**
  - **Request body.** It gains `today`, `from` and `to` (`YYYY-MM-DD`).
    - `from` and `to` are required. The handler returns 400 when either is missing, when the dates are malformed, when `to < from`, or when the span is over 31 days. Each case has its own i18n `messageKey`; follow the existing error pattern.
    - `today` is optional; when missing it falls back to the server's `now()` (D8).
    - Every frontend chat caller (`ChatPanel`, `GeneratePlanButton` and any other caller of the chat mutation) sends all three.
  - **`ChatRequest`** gains `Today`, `From` and `To` as `time.Time` values.
  - **Service.** `NewService` takes a `now func() time.Time`, defaulting to `time.Now` in main.
    - **Prompt contents.** `buildSystemPrompt` loads the plates in `[from, to]` and calls `ComposePrompt(p, plates, &DateRange{From, To}, prefs)`, plus a new today line, e.g.:
      ```
      Today is Monday, 2026-10-26.
      Planning window: Saturday 2026-10-31 to Friday 2026-11-06 (7 days).
      ```
      It then lists each date with its English weekday name.
    - **Plate wording.** Plates in the window are listed with date, slot and food names, and skipped plates are marked as skipped.
    - **Budget.** The prompt stays within `promptCharBudget`; existing truncation rules apply.
  - **`DebugSystemPrompt`** takes the same inputs and goes through the same path; the debug handler accepts the same query params.
  - **Prompt version.** Bump the agent `PromptVersion` to `"v2"`. The run's `window_start`/`window_end` are set from `from`/`to`.
  - **Frontend.** Send `today: todayISO()`, `from: range.from` and `to: range.to`. Keep the existing cache invalidation.
- **Tests first**
  - `prompt_test`: golden assertions on the date lines for a window that crosses a month boundary, today inside and outside the window, and an empty plate list.
  - `service_test`: run a chat with the fake client and assert that `fake.Requests()[0].System` contains the today line and every window date with its weekday.
  - Handler tests: 400 for missing `from`/`to`, for a bad date, for `to < from` and for a span over 31 days; 200 passes the dates through.
  - Vitest: the chat mutation posts `today`, `from` and `to`.
- **Verify:**
  ```bash
  cd backend && go test -race ./internal/domain/agent/ ./internal/transport/...
  cd frontend && bun run test src/lib/queries
  bun run e2e -- ai-chat   # runs in the ai-serial project
  ```

#### B2. Interim tool payload fields

Needs C2.

- **Files**: `internal/domain/agent/tools.go` and its tests (or the existing tool test file).
- **Behaviour**
  - `foodSummary` and `foodDetail` gain:
    - `favorite` and `cook_count`
    - `last_cooked_at` (date or null)
    - `per_portion`: `{kcal, protein, fat, carbs}`, rounded to one decimal place
    - `nutrition_complete` (bool), plus `missing_nutrition: [names]` when incomplete
  - `plateSummary` gains `skipped`.
  - Macros come from C2's `PerPortionDetailed`. In `list_foods`, compute them only for the returned page; the existing limit stays.
  - Update the tool descriptions so the model knows the fields exist.
- **Tests first**: for a favorite composed food with one incomplete leaf, the tool output has `favorite:true`, `nutrition_complete:false`, the missing name and the per-portion numbers; a skipped plate shows `skipped:true`.
- **Verify:** `cd backend && go test ./internal/domain/agent/`

### Track C: Import and nutrition

#### C1. Resolver: local catalogue first

- **Files**
  - `internal/domain/food/resolver.go` and `resolver_test.go`
  - new `internal/domain/food/normalize.go` and `_test.go`
  - a new food repo query only if no existing repo method can list leaf names (check `food_repo.go` first)
  - `main.go`, only if the OFF provider must now be passed in
- **Behaviour**
  - **Normalize.** `normalizeName` lowercases, trims and collapses spaces, and folds `ä→a`, `ö→o`, `ü→u`, `ß→ss` and other Latin diacritics. Both sides are compared with the same function, so "Möhre", "mohre" and "MÖHRE" all match.
  - **Search order:**
    1. Local leaves whose normalized name equals the normalized query, then those whose name starts with it, then those containing it. At most 5, with `existing_id` set and source `local`.
    2. FDC with the translated query, if a key is configured.
    3. OFF `SearchByName` with the original query.
    Results are de-duplicated by `(source, external id)`, and an exact local match is listed first.
  - **Matching.** `enrichExistingIDs` compares against the normalized **original** query, not the FDC description.
  - **No FDC key.** The resolver still returns local and OFF results (today it returns nothing).
  - **AI calls.** The translator and picker run only when FDC is queried.
- **Tests first**
  - A local "Möhre" leaf matches the query "möhre" before any FDC result.
  - No FDC key gives local plus OFF results.
  - An FDC result named "Carrots, raw" for the query "Karotte" does not overwrite a local "Karotte" match.
  - Normalize table tests.
- **Verify:** `cd backend && go test ./internal/domain/food/ -run 'Resolver|Normalize'`

#### C2. Nutrition completeness

- **Files**
  - `internal/domain/food/nutrition.go` and `nutrition_test.go`
  - `internal/transport/http/handlers/foods.go` (Nutrition handler around `:499`) and its test
- **Behaviour**
  - Add `PerPortionDetailed(ctx, id) (Detailed{Macros, Complete bool, Missing []string}, error)`. `PerPortion` keeps its signature and returns `Detailed.Macros`, so other callers don't change.
  - A leaf is incomplete when any of kcal, protein, fat or carbs per 100 g is nil. `Missing` holds the unique names of incomplete leaves, recursively through nested recipes, in a stable order.
  - Incomplete leaves still contribute 0, but the result is flagged.
  - GET `/api/foods/{id}/nutrition` adds `complete` and `missing`.
- **Tests first**
  - A leaf without kcal is incomplete and its own name is missing.
  - A recipe with one complete and one incomplete leaf lists one missing name.
  - A nested recipe propagates.
  - An all-complete recipe gives `complete:true` and `missing: []`.
  - Handler JSON includes the fields.
- **Verify:** `cd backend && go test ./internal/domain/food/ ./internal/transport/http/handlers/ -run 'Nutrition'`

#### C3. Parser fixes

- **Files**
  - `internal/adapters/jsonld/parser.go` and its test
  - `internal/domain/importer/parse_de.go` and `parse_de_test.go`
  - `importer/service.go`, only if `Draft` needs the new nutrition field
- **Behaviour**
  - **`recipeYield`** accepts a string, a number (`4` → `"4"`) or an array (first element that parses).
  - **JSON-LD `nutrition`.** Read `nutrition.calories` (e.g. `"520 kcal"`) into `Draft.SourceKcalPerServing *float64`. It is for display and comparison only and is never used for macros (§10.3).
  - **German parser:**
    - Glued numbers (`"100g Mehl"`, `"1,5kg Kartoffeln"`, `"250ml Milch"`) parse like the spaced forms.
    - The plural suffixes `(n)`, `/n`, `(s)`, `/s` and `(e)` are stripped from units and names and never become notes.
    - `Zehe`, `Zehen`, `Zehe(n)` and `Zehe/n` are a garlic clove: unit "Zehe", 5 g each. The name is the rest of the line, e.g. "Knoblauch".
  - **Quantity-less lines** ("Salz und Pfeffer", "etwas Öl"): first check how they flow through `Finalize` and the review UI today. If they block saving or need a manual choice, parse them as 0 g "nach Geschmack" lines. They stay visible, count 0 g toward macros, and are pre-resolved as optional. Document what was found in the step summary.
- **Tests first**: new rows in the parser tables for each case above, plus JSON-LD tests for a numeric yield, an array yield and the nutrition block.
- **Verify:** `cd backend && go test ./internal/adapters/jsonld/ ./internal/domain/importer/`

#### C4. Source URL, description and image on foods

Runs after A1 (migration number 00022).

- **Files**
  - Backend:
    - new `db/migrations/00022_food_source_description.sql`
    - `queries/foods.sql` and regenerated `sqlcgen/`
    - `food_repo.go` and its tests
    - new `migration_00022_test.go`
    - `domain/food` (entity fields; Create and Update pass-through)
    - `handlers/foods.go` and its tests
  - Frontend: `src/lib/api/foods.ts` types
- **Behaviour**
  - Add nullable `foods.source_url` and `foods.description`, read and written everywhere foods are read and written.
  - **Create request.** `foodRequest` gains `source_url`, `description` and `image_url` (create only).
  - **Image download.** After the row is created, the handler calls `imgStore.SaveFromURL(ctx, imageURL, "foods", id)` and stores the image path. On failure, the food is still created and the response carries `"warnings": ["image_download_failed"]`. The existing 5 MiB cap in httpfetch applies.
  - **Name conflict.** Creating a leaf whose name already exists returns 409 with the existing error key plus `existing_id`, so clients can reuse it.
- **Tests first**
  - The migration adds both columns.
  - The repo round-trips them, including nulls.
  - The create handler with `image_url` pointing at an `httptest` server serving a small PNG ends with `image_path` set.
  - The same with the server returning 404 still gives 201 plus the warning.
  - A duplicate leaf name gives 409 with `existing_id`.
- **Verify:** `cd backend && sqlc generate && go test ./internal/adapters/sqlite/ ./internal/domain/food/ ./internal/transport/http/handlers/`

#### C5. FDC refetch by id

- **Files**
  - `internal/adapters/fdc/dynamic_provider.go` (and `provider.go` if it is separate)
  - the port interface in `domain/food` that the lookup handler uses
  - `internal/transport/http/handlers/lookup.go` and its test
- **Behaviour**
  - Add `GetByID(ctx, fdcID int) (food.Candidate, error)` on top of `Client.GetFood`.
  - The refetch path at `lookup.go:184-185` parses `*existing.FdcID` and calls `GetByID`.
  - An unparsable id returns the existing validation error.
- **Tests first**: the handler test with a stub provider shows refetch calls `GetByID(1234)` and never `SearchByName("1234")`; the adapter test runs against an `httptest` FDC server.
- **Verify:** `cd backend && go test ./internal/adapters/fdc/ ./internal/transport/http/handlers/ -run 'Lookup|Refetch|GetByID'`

#### C6. Frontend import fixes

Needs C1, C2, C3 and C4.

- **Files**
  - `src/routes/import/index.tsx`
  - possibly a new `src/components/import/` with extracted subcomponents, if the file grows past what is readable
  - `src/lib/api/import.ts` and `foods.ts`
  - their tests
  - `e2e/import.spec.ts` and new `e2e/import-chefkoch.spec.ts`
  - i18n files
- **Behaviour**
  - **Candidates.** Every candidate is selectable (remove `disabled={!r.existing_id}`). Picking one without `existing_id` creates a leaf with the candidate's nutrition, source (`fdc`/`off`) and external id, then uses the new id. On a 409, it uses the returned `existing_id`.
  - **Create manually** reuses a same-name leaf through the 409 path. Errors surface through the standard mutation `toastError`; no more empty try/finally.
  - **Save rules.** Save is disabled until every ingredient row is resolved, and a counter says why ("2 ingredients need a choice"). Optional quantity-less lines from C3 count as resolved.
  - **Payload.** Saving sends `source_url`, `description` (no longer in `notes`) and `image_url`. A `warnings` entry shows a non-blocking toast.
  - **Review screen.** It shows the computed kcal per portion. If `SourceKcalPerServing` is present, it shows it alongside for comparison ("Source: 520 kcal"). If C2 reports incomplete nutrition, it shows a warning naming the ingredients.
  - **After save**, navigate to the recipe detail (`/components/$id`, from C7) instead of the editor.
- **Tests first**
  - Vitest (mocking `@/lib/api/*`, using `renderWithRouter` and shared fixtures):
    - A candidate without `existing_id` is clickable and triggers a create with its nutrition.
    - A 409 reuses `existing_id`.
    - A create error shows a toast.
    - Save is disabled while rows are unresolved, with the counter text.
    - The payload contains `description`, `source_url` and `image_url`, and `notes` is not set from the description.
  - **E2E `import-chefkoch.spec.ts`** (parallel project; it doesn't touch AI settings):
    - Fixture: a trimmed real chefkoch.de JSON-LD block (a German recipe with yield `4`, glued units, `Zehe/n`, `Zwiebel(n)` and "Salz und Pfeffer") as an inline fixture string.
    - Seed leaves with known per-100 g values for every ingredient, with unique suffixes and cleanup in `finally`. Post the HTML via `/api/import/extract`.
    - Resolve without touching any row except the auto-matches, then save.
    - Assert that kcal per portion on the detail page equals the hand-computed value (±1 kcal), with no manual fixes.
- **Verify:**
  ```bash
  cd frontend && bun run check
  bun run e2e -- import
  bun run e2e -- import-chefkoch --repeat-each=10 --workers=4
  ```

#### C7. Recipe detail (Phase 0 subset) and editor fields

Needs C2 and C4. It can run beside C6, but merge C6 after C7 or stub the navigation target.

- **Files**
  - new `src/routes/components/$id/index.tsx`
  - new `src/components/components/RecipeDetail.tsx` and its test
  - `ComponentCard` / `ComponentList`: card click opens the detail; edit stays reachable
  - `ComponentEditor` (source URL field, description field, completeness warning)
  - i18n files
- **Behaviour**, per D7:
  - Use display type (Newsreader) for the title, and the 68ch prose cap for notes, description and steps.
  - My notes is a distinct block; the description is muted below it.
  - The source link opens in a new tab with `rel="noopener noreferrer"`.
  - The completeness warning names the incomplete ingredients and links to `/ingredients/$id/edit`.
  - Favorite toggles through the existing mutation.
  - **Editor:**
    - Gains editable Source URL and Description fields.
    - Shows the same completeness warning.
    - Confirm that the image upload input does not set `capture`, so the phone camera roll works (UX §11 decision 3).
- **Tests first**:
  - Vitest: detail renders notes vs description, the source link, and the warning only when `complete:false`; the editor saves the new fields.
  - E2E: extend `components.spec.ts` so opening a card shows the detail and Edit reaches the editor.
- **Verify:** `cd frontend && bun run check && bun run e2e -- components`

### Track D: Infrastructure and docs

#### D1. Auto-generated secret key

- **Files**: `internal/config/config.go`, new `internal/config/secretkey.go` and `_test.go`, and `cmd/plantry/main.go` (call site only).
- **Behaviour**
  - `LoadOrCreateSecretKey(dir string) (string, error)` reads `<dir>/secret.key`, trimming whitespace.
  - If the file is missing, it writes 32 random bytes (`crypto/rand`) as 64 hex chars with mode 0600. Write to a temp file, then rename. Create the directory if it is missing.
  - `dir` is `filepath.Dir(cfg.DBPath)`. The env var `PLANTRY_SECRET_KEY` still wins and no file is written.
  - Log at info when a key is generated; never log the key.
  - The cipher setup at `main.go:83-90` then always has a key, so `crypto.NilCipher` stays only for tests.
- **Tests first** (`t.TempDir()`):
  - A new key has 64 hex chars and mode 0600.
  - A second call returns the same key.
  - Env set: the env value is used and no file is created.
  - An existing file with a trailing newline is trimmed.
  - An unreadable or corrupt (short) file returns an error.
  - The generated key satisfies `crypto.MinSecretLen`.
- **Verify:** `cd backend && go test ./internal/config/ && go build ./...`

#### D2. Docs cleanup

Last, after all other steps.

- **Files**: `README.md` (`:92`), `docker-compose.yml` (`:17`), `docs/ARCHITECTURE.md` (`:108` and `:1106`), `backend/CLAUDE.md`, `frontend/CLAUDE.md`.
- **Content**
  - Remove `PLANTRY_AUTH_PASSWORD`.
  - Document that the secret key is auto-generated in the data directory, that the env var overrides it, and the format (any string ≥ `MinSecretLen`; generated keys are 64 hex chars). The "32-char hex" note in `backend/CLAUDE.md` is wrong.
  - Document `ai_runs` and how to query it.
  - Rewrite the stale `frontend/CLAUDE.md` route list:
    - `/` is the planner; it does not redirect to `/day/today`.
    - Add `/shop` and `/components/$id`.
  - Rewrite the design-system section: new tokens, Newsreader and Source Sans 3, the legacy aliases and when to use which names.
  - If the user approved the impeccable `document` pass, add a link to `DESIGN.md`.
- **Verify:** `grep -rn --exclude=PHASE_0_PLAN.md 'AUTH_PASSWORD\|Manrope\|Inter Variable\|32-char hex' README.md docker-compose.yml docs backend/CLAUDE.md frontend/CLAUDE.md` returns nothing, apart from historical mentions inside the specs.

### Track E: Frontend design and navigation

#### E1. New tokens and fonts

- **Files**
  - `frontend/package.json` and `bun.lock`: add `@fontsource-variable/newsreader` and `@fontsource-variable/source-sans-3`; remove `@fontsource-variable/inter` and `@fontsource-variable/manrope`
  - `frontend/src/index.css`
  - new `frontend/src/test/tokens.test.ts`
- **Behaviour**
  - **Colours.** Define the §3.1 tokens for light and dark:
    - `--ground`, `--panel`, `--panel-sunken`, `--ink`, `--ink-muted`, `--line`, `--accent`, `--on-accent`
    - `--batch-1..6` and `--batch-1..6-tint`
    - `--success`, `--warning`, `--danger`
    Keep the existing dark-mode mechanism (check whether it uses a `.dark` class or a media query, and keep it).
  - **Tailwind colours.** Expose them through `@theme inline`: `bg-ground`, `bg-panel`, `bg-panel-sunken`, `text-ink`, `text-ink-muted`, `border-line`, `bg-accent`, `text-on-accent`, `bg-batch-1` and so on, and `text-success`, `text-warning`, `text-danger`.
  - **Aliases (D2):**
    - shadcn variables:
      - `--background`→ground
      - `--foreground`→ink
      - `--card`/`--popover`→panel
      - `--primary`→accent
      - `--primary-foreground`→on-accent
      - `--secondary`/`--muted`→panel-sunken
      - `--muted-foreground`→ink-muted
      - `--border`/`--input`→line
      - `--ring`→accent
      - `--destructive`→danger
      - sidebar variables to the matching rail tokens
    - Every legacy Material token in `index.css` maps to the nearest new token. Write the full mapping as a comment table at the top of the alias block.
    - `--ai-accent*` maps to the accent.
  - **Fonts.** Body and UI use `Source Sans 3 Variable`. The existing heading/display font utility uses `Newsreader Variable` (opsz axis), so wherever headings used Manrope they now get Newsreader. Add a `tabular-nums` helper class if none exists.
  - **Type scale:** body 0.9375rem, small 0.8125rem, caption 0.75rem, h3 1.1875rem, h2 1.4375rem, h1 1.75rem (Newsreader 600). No all-caps label utilities: remove `uppercase tracking-*` only where it is defined as a token or utility in `index.css`, not in components.
  - **Radii:** `--radius` 8px for buttons; add `--radius-cell` 4px, `--radius-sheet` 16px and `--radius-tag` `3px 10px 10px 3px`.
    - Inputs keep the shadcn `--radius`-derived radius in Phase 0; the 4px input radius from UX §3.3 is applied when screens are rebuilt (same reasoning as D2).
  - **Shadows:** one soft shadow for sheets and popovers.
- **Tests first** (`tokens.test.ts`, which reads `index.css` as text):
  - Every §3.1 token is defined in both themes with the exact hex values.
  - WCAG contrast ≥ 4.5 for: ink/ground, ink/panel, ink-muted/ground, ink-muted/panel, on-accent/accent, and ground on every batch colour, in both themes.
  - Every state colour against ground is ≥ 3.
  - No `Inter` or `Manrope` font-family remains.
- **Verify**
  - `cd frontend && bun install && bun run check && bun run build`
  - With the app running (`bun run dev` plus the backend), the `agent-browser` skill takes screenshots of the planner, `/components`, `/ingredients`, `/import` and `/settings` at 375, 820, 1180 and 1440 px in light and dark. Compare them against the "Chosen" section of `docs/design/visual-directions.html`.
  - Fix the obvious breakage caused by aliasing (unreadable text, invisible borders) in `index.css` only.
  - Run an impeccable `critique` by reading project context directly (D9). Record findings in the step summary; component-level fixes belong to later phases unless they are breakage.
  - The per-screen impeccable `audit` (UX §9) is covered by the final review in §4 (`frontend-review` on `/`, `/components/$id`, `/import`), not at E1.

#### E2. Four-destination navigation

- **Files**
  - `src/components/shell/SideNav.tsx`, `SideNavItem.tsx`, `MobileBottomNav.tsx`, `TopBar.tsx`
  - new `src/routes/shop/index.tsx`
  - `src/routes/components/index.tsx`, or the component list header: links to Ingredients and Import
  - `src/components/command/CommandPalette.tsx`: make sure Calendar, Presets, Archive, Ingredients and Import are entries
  - i18n files and their tests
  - e2e: `shell`, `catalog-grid`, `archive-redirect`, `components`, `day-detail`, `planner-picker`, `inventory-grid`
- **Behaviour**, per D1 and UX §2.1:
  - **Rail** (≥768 px): Today, Plan, Recipes and Shop with labels; Settings pinned at the bottom.
  - **Phone:** a bottom bar with the same four, and a Settings gear in the top bar.
  - **Today** links to `/day/$date` with `todayISO()`. Compute it in the click handler or a `useState` initializer, not during render (React compiler rule).
  - **Active states:**
    - Plan is active on `/`.
    - Recipes is active on `/components*`, `/ingredients*` and `/import`.
    - Today is active on `/day/*`.
  - **`/shop`** computes the current window the way `routes/index.tsx` does (reuse the helpers in `planner-window.ts`; extract a small shared hook if the logic is duplicated) and renders `ShoppingPanel` full-width with a page title.
  - Calendar, Presets and Archive leave the nav but keep their routes and stay reachable via ⌘K.
  - Use sentence-case labels and Lucide icons at stroke 1.5.
- **Tests first**:
  - Vitest: SideNav renders exactly four destinations plus Settings; the active state for `/ingredients` is Recipes; MobileBottomNav renders four tabs; TopBar shows the gear below 768 px.
  - Update the e2e specs to navigate through the new entries or by direct URL. `shell.spec` asserts the four destinations on desktop and phone viewports.
- **Verify:**
  ```bash
  cd frontend && bun run check
  bun run e2e -- shell catalog-grid archive-redirect components day-detail planner-picker inventory-grid
  ```

#### E3. Shopping range anchored on the viewed window

- **Files**
  - `src/components/planner/ShoppingPanel.tsx` and its test
  - `src/routes/index.tsx` (`:118` fallback)
  - `src/components/settings/tabs/PlanTab.tsx` (`:71` fallback)
  - i18n files
- **Behaviour**
  - **Presets** are computed from the `range` prop, not `new Date()`:
    - "This window" is `range.from`→`range.to`.
    - "Next window" is the following window of the same length.
    - "Rest of this window" is `max(range.from, today)`→`range.to`, and is shown only when today lies inside the window.
  - **Default selection:** "Rest of this window" when today is inside the window, otherwise "This window".
  - **Clock:** today comes from a `useState` initializer, keeping render pure.
  - **Fallback:** both frontend shopping-day fallbacks become `"0"`, matching the backend default in `specs.go`.
- **Tests first**:
  - Viewing next week's window gives a default starting at that window's start, not today.
  - Viewing the current window on its third day gives a default from today.
  - "Next window" gives the next 7 days.
  - The fallback is `"0"` when the setting is missing. Use `vi.setSystemTime`.
- **Verify:** `cd frontend && bun run test src/components/planner/ShoppingPanel && bun run check`

---

## 4. Waves and parallelism

The same file must never be edited by two parallel steps. The shared files are:
- `main.go`: A4, D1, and possibly C1 and B1
- `en.json` and `de.json`: B1, C6, C7, E2, E3. Each step adds keys under its own namespace, so merges are mechanical.
- `routes/index.tsx`: E3, and possibly E2 if a window hook is extracted
- `sqlcgen/`: A1 and C4; regenerate on conflict

| Wave | Steps (parallel within a wave) | Notes |
|---|---|---|
| 0 | Commit this plan and the UX §11 edit | Docs PR |
| 1 | A1, A3, C1, C2, C3, C5, D1, E1, E3 | No shared hand-written files. C1 and C2 are different files in `domain/food`. |
| 2 | A2, B2, C4, E2 | A2 needs A1 (store port) and A3 (fake). B2 needs C2. C4 needs A1 (migration order). E2 builds `/shop` on E3's props. |
| 3 | A4 → B1 (sequential); C7 → C6 (C7 first, or in parallel with a stubbed nav target) | A4 and B1 both edit `agent/service.go` and `main.go`. C6 needs C1–C4. |
| 4 | D2, then the full DoD check and review | |

**Final verification (DoD):**

```bash
cd backend && go test -race ./... && go vet ./... && golangci-lint run
cd frontend && bun run check && bun run e2e
```

Then:
- Run the `frontend-review` skill on the planner (`/`), `/components/$id` and `/import`.
- Run the `go-reviewer` agent on the whole backend diff.
- Do a manual chat run with the fake provider and check `ai_runs` as in A4.
- Do a manual chefkoch import, with a real URL only if the user agrees to a network call.

---

## 5. Out of Phase 0 (explicitly)

- Cost table and pricing (deferred to Phase 3; D3)
- `ai.price_overrides`
- Runs API and insights page
- Per-food count portions (deferred to Phase 1; D6)
- Diet class (deferred to Phase 1; D6)
- Route renames (`/recipes`, `/today`, `/plan`); D1
- Today, Shop and Board redesigns (Phase 2)
- Cook now (Phase 2)
- Removing calendar, presets and archive routes (they leave the nav only)
- Renaming the ~800 legacy utility usages (D2)
