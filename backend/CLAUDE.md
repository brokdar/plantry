# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Backend — Plantry

Go binary (module `github.com/jaltszeimer/plantry/backend`). Must cross-compile to ARM with no CGO — use `modernc.org/sqlite` (pure Go), never `mattn/go-sqlite3`.

## Commands

All commands run from `backend/`:

```bash
go build ./...                        # compile
go test ./...                         # all tests
go test -run TestName ./path/...      # single test
go vet ./...                          # vet
golangci-lint run                     # lint
sqlc generate                         # regenerate from queries + schema
go test -race ./...                   # race detector — run before feature complete
scripts/check-sqlc-usage.sh           # every sqlc query needs a non-test caller
```

`scripts/check-sqlc-usage.sh` fails when a `-- name: X` in `internal/adapters/sqlite/queries/*.sql` has no `.X(` in a non-test `.go` file outside `sqlcgen/`. It runs in CI and in the repo-root `scripts/gate.sh`; delete the query instead of keeping it unused.

## Required Skills

Load these before writing or modifying backend code:

- **`golang-patterns`** — Go code
- **`golang-testing`** — Go tests

## Architecture — Hexagonal-Lite

Data flow: `transport → domain ← adapters`

```
backend/
├── cmd/plantry/main.go              # entrypoint — wires everything together
├── cmd/plantry-eval/                # offline eval command (see Plan generation)
├── db/
│   ├── migrations/                  # goose SQL migrations (+ Go migration files)
│   └── schema_patches/              # additive schema additions outside numbered migrations
└── internal/
    ├── adapters/
    │   ├── sqlite/                  # sqlc repo implementations
    │   │   ├── queries/             # *.sql files with sqlc annotations
    │   │   └── sqlcgen/             # generated — never edit by hand
    │   ├── anthropic/               # Anthropic SDK — preset generation, AI features
    │   ├── openai/                  # OpenAI — food lookup
    │   ├── fdc/                     # FoodData Central API
    │   ├── off/                     # Open Food Facts API
    │   ├── imagestore/              # image storage
    │   ├── httpfetch/               # generic HTTP fetch adapter
    │   ├── llmresolver/             # selects LLM provider at runtime
    │   ├── jsonld/                  # JSON-LD parsing (recipe import)
    │   ├── fake/                    # in-memory fakes for unit tests
    │   └── crypto/                  # ID generation helpers
    ├── domain/                      # pure business logic — no I/O
    │   ├── food/                    # Food aggregate (ingredients + portions)
    │   ├── plate/                   # Plate aggregate: plates, components, cook sessions, events
    │   ├── planview/                # read models for /api/plan and /api/today
    │   ├── planning/                # plan generation: checker, choosers, runner, re-plan
    │   ├── diet/                    # diet classes (pure)
    │   ├── window/                  # plan window arithmetic (pure)
    │   ├── actor/                   # who made a change (user / agent) in ctx
    │   ├── slot/                    # Slot aggregate (planner day slots)
    │   ├── preset/                  # Preset aggregate (saved day / week compositions)
    │   ├── nutrition/               # nutrition calculation
    │   ├── shopping/                # shopping list derivation
    │   ├── profile/                 # user profile
    │   ├── settings/                # app settings
    │   ├── availability/            # cell availability (plan / routine / no plate)
    │   ├── setup/                   # first-run setup status
    │   ├── agent/                   # AI agent aggregate
    │   ├── airun/                   # AI run log (metered LLM client)
    │   ├── llm/                     # LLM service interface
    │   ├── importer/                # recipe import orchestration
    │   ├── feedback/                # plate feedback
    │   └── units/                   # unit conversion
    ├── eval/                        # offline eval suite behind cmd/plantry-eval (scenarios, library)
    ├── transport/http/
    │   ├── router.go                # chi route registrations
    │   ├── handlers/                # thin HTTP translation, one file per aggregate
    │   ├── middleware/              # request middleware
    │   └── sse/                     # server-sent events (AI streaming)
    ├── config/                      # env var loading
    ├── testhelper/                  # NewTestDB, HTTP test fixtures
    └── webui/                       # embeds frontend dist/
```

**Aggregates** are plain exported structs. `*.Service` holds business logic and takes repo interfaces as constructor args — never concrete types.

**sqlc workflow:** edit `queries/*.sql` → run `sqlc generate` → implement/update `*_repo.go` using generated functions. Never write raw SQL outside of query files.

**Safe table rebuild.** Dropping a table with foreign keys on cascade-deletes its dependants (00014/00016 did on old installs). Rebuild tables (e.g. to add `AUTOINCREMENT`, as 00030 for `plates`/`cook_sessions` and 00033 for `presets`) in a Go migration registered with `goose.AddMigrationNoTxContext`: pin one connection with `db.Conn`, `PRAGMA foreign_keys = OFF` on it, copy in one transaction, run `PRAGMA foreign_key_check` before commit, restore `foreign_keys = ON` afterwards, and recreate triggers the drop removed. Give sqlc the end-state schema in `db/schema_patches/`.

**Migrations** use goose. Numbered SQL files in `db/migrations/`. Go migration files (`.go`) handle complex data transformations. Migrations run automatically at startup (`goose.Up` in `main.go`) — no manual step needed.

## Environment Variables

All optional unless noted. Prefix: `PLANTRY_`.

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `DB_PATH` | `/data/plantry.db` | SQLite file path |
| `IMAGE_PATH` | `/data/images` | Image storage directory |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `FDC_API_KEY` | — | FoodData Central API key |
| `AI_PROVIDER` | — | `openai` / `anthropic` / `fake` |
| `AI_MODEL` | — | **Required** when `AI_PROVIDER` is set |
| `AI_API_KEY` | — | **Required** for `openai` / `anthropic` |
| `AI_RATE_LIMIT_PER_MIN` | `10` | AI request rate cap |
| `AI_FAKE_SCRIPT` | — | JSON script path; **required** for `fake` provider |
| `SECRET_KEY` | `<dir of DB_PATH>/secret.key` | Encrypts API keys stored in the DB. Any string ≥ 32 chars (`crypto.MinSecretLen`) |
| `DEV_MODE` | `false` | Exposes debug-only endpoints |
| `OFF_DISABLED` | `false` | Skip Open Food Facts lookups (hermetic e2e runs) |

**Secret key.** `config.EnsureSecretKey` runs at startup. If `PLANTRY_SECRET_KEY` is set it wins and no file is written. Otherwise the key is read from `secret.key` next to the database (`filepath.Dir(PLANTRY_DB_PATH)`), or generated there on first start: 32 random bytes as 64 hex chars, mode 0600. A key file shorter than 32 chars fails startup. In Docker the `/data` volume holds both the DB and the key; losing the key makes stored API keys undecryptable. `crypto.NilCipher` exists only for tests.

## Settings

Key-value rows in `app_settings`. The registry in `domain/settings/specs.go` lists every key with its kind (string, int, bool, enum), default and range; unknown keys are rejected. A value resolves as DB → env var → default, and `GET /api/settings` reports each value's `source`. `PUT /api/settings/{key}` (body `{value}`) writes a DB override; `DELETE` removes it.

| Key | Default | Notes |
|---|---|---|
| `ai.provider`, `ai.model`, `ai.api_key`, `ai.rate_limit_per_min`, `ai.fake_script`, `fdc.api_key` | env | Fall back to the `PLANTRY_*` env vars; the API keys are stored encrypted |
| `plan.anchor` | `next_shopping_day` | `today` / `next_shopping_day` (00028 retired `fixed_weekday` and `plan.week_starts_on`) |
| `plan.shopping_day` | `0` | 0 = Monday … 6 = Sunday |
| `setup.completed` | `false` | Set by `POST /api/setup/complete` |
| `plan.max_cook_occasions_per_day`, `plan.batch_cooking`, `plan.default_keep_days`, `plan.freezing_enabled`, `plan.freezer_keep_days`, `plan.variety_weeks`, `plan.explore_per_week`, `discovery.daily_enabled` | see `specs.go` | Stored and edited in Settings; read by later phases |
| `ai.run_retention_days` | `180` | 7–3650; see AI run log |
| `ai.price_overrides` | `{}` | JSON map model id → `{in, out, cache_read?, cache_write?}` USD per 1M tokens, validated by `airun.ParsePriceOverrides` (`Spec.Validate`); see AI run log |

The profile (`domain/profile`) also carries `diet_type`, `meat_days_per_week_max` and `household_portions` (00023).

## Availability

Migration `00024_availability.sql`, package `domain/availability`. A cell is (date, slot) and its mode is `plan`, `routine` (needs a `preset_id`; a label only for now, nothing applies the preset) or `none` (No plate). It resolves as per-date override → weekday default (weekday 0 = Monday) → `plan`. `plan` defaults are never stored.

- `GET /api/availability?from=&to=` — resolved cells with `source` (`override` / `default` / `fallback`), at most 62 days.
- `GET` / `PUT /api/availability/defaults` — `{items: [{weekday, slot_id, mode, preset_id}]}`; PUT replaces the defaults of all active slots.
- `PUT /api/availability/{date}/{slotId}` — `{mode, preset_id?, note?, remove_plate?}`. `none` on a cell with a plate is `409 error.availability.cell_has_plate` unless `remove_plate` is set; then the plate is deleted through `plate.Service.Delete` in the same unit of work and the response `{cell, removed}` carries the `PlanSnapshot` that restores it (`POST /api/plan/restore`; `removed` is null when nothing was removed). `DELETE` clears the override.

A cell holds at most one plate: migration `00025_plates_cell_unique.go` turned the old per-plate skip flag into `none` overrides (with its note), merged duplicates, dropped the flag column and made `(date, slot_id)` unique. Creating or moving a plate into a `none` cell is `409 error.plate.cell_unavailable`; into an occupied cell, `409 error.plate.cell_occupied`. The agent sees `none` and `routine` cells and must not plan them.

## Plan window and read models

Package `domain/window` (pure): a window is the 7 days starting on `plan.shopping_day`. `Containing(date, sd)` starts on the latest shopping day ≤ date; `Anchored(today, anchor, sd)` is the window containing today (`today`) or starting on the first shopping day ≥ today (`next_shopping_day`).

Package `domain/planview` builds the read models; it never writes and takes its dates as arguments. Plates load with one `ListByDateRange` call for the whole range and sessions with `CookSessionRepo.ListInRange` (two queries), never per plate.

- `GET /api/plan?start&today` — the window at `start` (must be a shopping day, else `400 error.plan.invalid_start`) or the anchored one: `window`, active `slots`, `cells` (date × active slot with availability and plate), `sessions` (cooking in the window or linked to a plate in it), `days` (per-eater macros with `complete`/`missing`), `targets` (null without a kcal target), `stats`, `issues` (`planning.Check` over every cell of the window, filtered to H1, H2, H5 and H8, with diet classes from `food.Service.DeriveDiet`; `missing_plate` only while the window has drafts and only for cells dated today or later), `draft` (`{plates, sessions, run_id, notes, chooser}` of the newest run with drafts in the window, read through the `RunNotes` port; null without drafts) and `running_run_id` (from the `Progress` port; null while none is wired). The recipe list H8 needs loads only when the window has drafts.
- `GET /api/today?date&today` — confirmed rows only (D26); `verdict` (`draft_only` when the date has no confirmed plate and the window has drafts, `no_plan`, `no_plates`, `cook`/`quick_cook` for the uncooked main with most portions, ≤ 20 min is quick, else `nothing_to_cook`), `cook`, `meals`, `reminders` (`defrost`, `use_by`), `rate`, `plan_banner` and `setup`.
- `today` defaults to the server's local date; `date` defaults to `today`.

## Unit of work

`domain.Transactor` (`domain/transactor.go`, D21) is the port; `sqlite.TxRunner` (`adapters/sqlite/tx.go`) implements it. `InTx(ctx, fn)` runs `fn` in one transaction whose `*sql.Tx` travels in `fn`'s ctx, and every repository called with that ctx joins it. A nested `InTx` joins the outer one: there are no savepoints, so a failed nested call leaves its partial writes in the outer transaction and the caller must return the error. Multi-write operations (plate mutations, availability `none` with `remove_plate`, preset apply) run inside one `InTx`, and so do the `/api/plan` and `/api/today` read models, so a concurrent write cannot tear them.

Every domain sentinel's i18n message key lives in `domain.MessageKey` (`domain/errors.go`; the plate quantity sentinels in `plate.MessageKey`); the HTTP error map adds only the status, and the agent's tool errors use the same keys. A test fails when a sentinel in `errors.go` has no key or no status.

## Plate aggregate and write path

`plate.Service` (`domain/plate/service.go`, built with `plate.NewService(plate.Deps{...})`) is the single write path for plates, plate components and cook sessions. Handlers, the agent tools and preset apply/copy all go through it.

- A cell (date, slot) holds at most one plate. A plate has `eaters` (default: the profile's household portions) and ordered components; a component is a food with `portions` per eater, or for a leaf `amount` + `unit`.
- A composed component is linked to a cook session. Without a given `CookSessionID` a new component gets a fresh session at the plate's own cell. Mutators: `Create`, `Update`, `Delete`, `ClearRange`, `AddComponent`, `UpdateComponentQuantity`, `SwapComponent`, `RemoveComponent`, `Apply`.
- `Delete`, `ClearRange` and session deletion return a `PlanSnapshot` that restores them; `Apply(snapshot, ApplyOptions)` writes one (D22). `Apply` decides every plate's placement before it writes anything, so a conflict (an occupied cell without overwrite, a duplicate cell in the snapshot) fails the whole call. Snapshot refs are strings (`"s1"`), snapshot components carry `sort_order`, snapshot plates an optional `plate_id`. Preset apply and copy-week build snapshots and call `Apply`.
- `DELETE /api/plan?from&to&slot_id` is slot-scoped (`ClearRange`).

### Sessions and D11

A `CookSession` (`domain/plate/session.go`) says "food X is cooked at (cook date, cook slot), producing `portions`", with `frozen_portions`, `cooked_at` and a note. Components link to it as `fresh` (own cell), `leftover` (later within keep days) or `frozen`; links are ordered in cell order (date, slot `sort_order`, slot id; D12). `plate.BuildView` / `SessionView` add links, consumption and warnings (`leftover_before_cook`, `past_keep_days`, `deficit`, `frozen_deficit`; D13).

D11 (`plate.Sync`): when a change alters what a session's links consume, a session that exactly covered its links before still covers them exactly after; one with a surplus or deficit keeps its portions; frozen portions never exceed portions. The service tracks every session a mutation touches (`tracker`) and settles once at the end: a session without links is deleted, the others follow D11. Explicit edits (`SetSessionPortions`, `SetCooked`, `AddLeftovers`, `DeleteSession`) always win over D11. Freezing needs `plan.freezing_enabled` and a recipe not marked unfreezable (D14). `LeftoverCells` suggests the next cells for leftovers.

## Units and rounding

`domain/units` (`round.go`): `Display(amount, unit)` rounds an exact amount for reading (D26). Metric mass/volume work in g/ml, rounded to 1 below 100 and to 5 from 100; from 1000 they switch to kg/l only for clean multiples of 100 (1500 g becomes 1.5 kg, 1235 g stays). tbsp/tsp round to 0.5; everything else, counts included, stays exact. `ToBase` converts metric units to g/ml so sums add up. `Buy` rounds a count unit up to whole units. `CountGrams` is the only count-to-gram table. Amounts are stored and summed exact; round only for display.

## Food history and list filters

Eating history is derived, never stored (D16): `food.Service.Derive(ctx, today, foods...)` sets `LastEaten` and `TimesCooked` on composed foods and `UsedInCount` on leaves, with one query per kind (sqlc `FoodHistory`, `UsedInCount`). Plates marked `not_eaten` and draft rows do not count.

`GET /api/foods` filters: `max_minutes`, `not_eaten_weeks`, `created_within_days`, `today`, `sort=name|last_eaten|created` (`last_eaten` ignores favorites). Recipes carry `last_eaten` (`YYYY-MM-DD`), `times_cooked` and `per_portion` (batched `PerPortionDetailedMany`); leaves carry `used_in_count`. Plate feedback statuses are `loved`, `disliked`, `not_eaten`; others are `400 error.feedback.invalid_status`. `GET /api/foods/macros` serves the plate sheet's macro preview. `GET /api/foods/{id}/scaled?portions=` (required, 0 < p <= 100) returns the scaled ingredient list.

## Diet class

Package `domain/diet` (no domain imports): `Class` (`vegan` < `vegetarian` < `pescetarian` < `omnivore`, plus `unknown`), `Order`, `Valid`, `Max` (any unknown child, or none, gives unknown), `Compatible(class, dietType)` (class ≤ type; unknown never) and `Classify(name)`, a 3-stage data-driven en/de keyword classifier: phrases like "Kokosmilch" add their class and are cut out of the name, then markers like "vegan" or the animal keywords run on the rest, and the result is the max of everything found (else a plant keyword → vegan, else unknown); umlauts and ß are folded.

Migration 00035 adds `foods.diet_class`, an override only (NULL = derived, nothing backfilled; D28). `food.Food` carries `DietOverride` (stored) and the derived `DietClass`/`DietClassSource` (`auto` leaf name, `user` override, `derived` recipe max over its children). `food.Service.Derive` sets them (`DeriveDiet`: one `LoadTrees` for all recipes without an override); `food.SetDietClasses(nodes)` does it on an already loaded tree.

- Every food JSON has `diet_class` and `diet_class_source`; scaled ingredients and import draft ingredients carry `diet_class`.
- `GET /api/foods?diet=<class>` keeps foods with class ≤ it, unknown excluded. The class is not stored, so the service filters after the SQL query and pages afterwards.
- `PUT /api/foods/{id}/diet-class {class | null}` sets or clears the override (both kinds); error `error.food.invalid_diet_class`.

Plate feedback (spec §6.6): loved/disliked move the plate's recipe tags into `likes`/`dislikes` and out of the opposite list; role names and `explore` are skipped.

## Plan generation

Package `domain/planning` turns "plan my week" into a checked draft: context assembled in Go, one proposal from a chooser (LLM or heuristic), the checker, a bounded repair loop, then drafts persisted. The HTTP handlers (`handlers/plan_generate.go`) are its only caller; the chat agent does not use it.

- **Checker.** `planning.Check(State, Target) []Issue` applies the hard rules H1–H8 to a pure `State`; `Merge(State, Target, Proposal)` lays a proposal over the fixed context (proposed plates get negative ids, sessions keep their key); `ComputeScore` and `ToSnapshot` complete the pipeline. Issue codes beyond the spec's: H1 `extra_plate`, `missing_plate`; H6 `confirmed_in_target`, `session_outside_target`; H7 `unknown_food`, `unknown_session`, `not_composed`, `session_food_mismatch`, `duplicate_session_key`, `invalid_portions`, `plate_without_components`, `plate_outside_target`, `plate_on_confirmed_cell`. H8 (variety) judges only the **proposed** (unconfirmed) cooks of a run: confirmed rows and routine cells are never subject to it, and it relaxes to the `variety_relaxed` warning when fewer eligible mains exist than cooks. `GET /api/plan` runs the same `Check` and filters to H1, H2, H5 and H8.
- **Scopes (D6).** `window` (generate), `day`, `plate`, `side` and `empty_cells`; `Scope.From`/`To` bound `empty_cells` (the re-plan). Everything outside the scope and everything confirmed is fixed context. Use the name `empty_cells`: the frontend legacy gate bans `fill_empty`/`replace_all`.
- **Endpoints.** `POST /api/plan/generate {start, chooser?, instruction?, eaters_by_date?, include_new_recipe?}` and `POST /api/plan/regenerate {scope, start, chooser?, instruction?}` return `202 {run_id}` (`?today=` is the client's date). `GET /api/plan/runs/{id}` returns status, `stage`, counts, issues, score and window; the client polls it, there is no SSE (D9). Regenerate keeps each day's eaters (guests) and never asks for a new recipe. Fewer than `setup.RecipeTarget` composed recipes is `error.plan.not_enough_recipes`; a second run on a window is `error.plan.generation_running`. `POST /api/plan/accept` and `/discard` `{from, to}` act on the window's draft closure (accept returns `{plate_ids, session_ids}`, the body of `POST /api/plan/unaccept`; discard returns `{removed}`, which `POST /api/plan/restore` brings back as drafts); `POST /api/plates/{id}/confirm|unlock` and `/api/cook-sessions/{id}/confirm|unlock` move one row's closure. Confirmed wins wherever a link joins different states.
- **Runner.** A generation is asynchronous: `start` validates, picks the chooser, reserves the window in `planning.Progress` (the in-memory registry; one run per window), creates the `ai_runs` row and runs in a goroutine on a **detached ctx** with a 3 minute timeout (`runTimeout`). A panic ends the run as an error instead of the process. Stages (D35): `resolving`, `choosing`, `checking`, `repairing`, `saving`. `Progress.Running(start)` feeds `running_run_id` on `GET /api/plan`, which is how a reloaded board re-attaches. `Service.Wait` lets shutdown wait for running runs; `Deps.Go` lets tests run synchronously. At startup `FailInterruptedRuns` ends `running` rows left by a crash as errors (logs `ai.run.interrupted`). A run with no open `plan` cell skips the chooser (routines are still applied; it ends `ok`, cost null).
- **Chooser selection.** `Deps.Choosers` maps `airun.ChooserLLM` / `ChooserHeuristic`. A request **without `chooser`** uses the LLM only when a provider resolves (the chooser's `Ready`), else the heuristic. Explicit `llm` without a provider is `503 error.ai.provider_missing`; a provider that cannot be built is `502 error.ai.provider_error`; a reply that never yields a readable plan ends the run `error.ai.invalid_plan` (502). The e2e backend always has the fake provider, so heuristic e2e requests must pin `chooser: heuristic`.
- **Heuristic** (`heuristic.go`, run-log version `h1` = `planning.HeuristicVersion`): deterministic, no AI. The first day cooks at its earliest cell; later days cook at dinner and the earlier cells eat leftovers (sessions grow within keep days). Bump the version when it changes what it picks.
- **LLM chooser** (`llmchooser.go`, `prompt.go`): see AI run log. Prompt version `p1`; bump `PromptVersion` on every prompt change. The repair loop lives inside the chooser (at most 2 repairs, stage `repairing`).
- **Persisting.** Generated plates and sessions are written through `plate.Service` as `draft` with `actor.Generator` and the run's `ai_run_id`. A generator plate linked to a confirmed session is created confirmed, so never delete it in cleanup; count it as kept. `plate.Service.RemoveDrafts`, `DraftClosure` and `PlateClosure` support regenerate.

### Mid-week re-plan

`GET /api/plan/replan/preview?from&to[&none=date:slot…][&today]` returns `{none_cells, confirmed_affected, spare}`: the No-plate cells that hold plates, the confirmed plates a re-plan would remove, and the batches that would cook more than is still eaten (`spare[]` with `portions`, `freezable`, `mutable`; a batch is mutable only when it is not cooked and cooks on or after `from`, and only a mutable one can be frozen or reduced).

`POST /api/plan/replan {from, to, none_cells, unlock_confirmed, spare: freeze|keep, regenerate, chooser?, instruction?}` returns `{removed, run_id}`. **`none_cells` is part of the contract**: the re-plan writes the No-plate overrides itself in the same transaction, because the availability API refuses `none` on an occupied cell. Confirmed plates need `unlock_confirmed`, else `409 error.plan.confirmed_affected` lists them; `from` before `?today=` is `400 error.plan.replan_past`. Days before `from` and cooked portions are never touched. The server's only sources of "today" are `?today=` and its clock. The window is reserved from before the removal until the run starts, so no other generation slips in between; if the run fails to start after the commit, the error is logged and `run_id` is null. `removed` alone cannot undo a re-plan (the overrides and the new run are not in it), so a re-plan has **no Undo**.

`plate.Service.RemovePlates(ctx, ids, keep)` is the removal primitive: one unit of work; each session in `keep` ends with exactly its (frozen) portions and survives without links, so a cooked batch is never deleted; other sessions follow D11. An uncooked session that loses the plate on its cook cell but keeps other links **moves its cook to the earliest remaining plate** in the same transaction (a session update event; this can add an H2 on the new day, accepted). A missing plate is `ErrNotFound` and nothing is removed.

### Diet classes

See Diet class. Planning uses `domain/diet` (`Classify`, `Compatible`, `Max`) through `food.Service.DeriveDiet`; H5 compares each food's class with `profile.diet_type`, and an unknown class is a warning, not a violation.

### Offline eval

`internal/eval` and `cmd/plantry-eval` run plan generation offline (spec §8.4): an embedded 30-recipe library (`library.json`) and seven scenarios (`internal/eval/scenarios/*.json`), each run on a fresh in-process database copied from a migrated template, through services wired like `main.go`. A scenario's expectations: `min_plates`, `no_plate`, `kept_confirmed`, `unchanged_before`, `frozen_spare`, `min_repeats`.

```bash
go run ./cmd/plantry-eval                                   # all scenarios, heuristic chooser, 3 runs each
go run ./cmd/plantry-eval -scenarios family_daily,away_thu_fri -runs 5
go run ./cmd/plantry-eval -chooser llm -prompt-version p1   # scripted LLM mode, no network
PLANTRY_EVAL_LIVE=1 PLANTRY_AI_PROVIDER=anthropic PLANTRY_AI_API_KEY=... \
  go run ./cmd/plantry-eval -chooser llm -model <model>     # live: real provider, costs money
go run ./cmd/plantry-eval -compare earlier.jsonl            # per-component diff against an earlier report
```

Flags: `-scenarios` (comma-separated, default all), `-chooser` (`heuristic` or `llm`), `-runs` (default 3), `-model`, `-prompt-version`, `-out` (JSONL, default `plantry-eval.jsonl`, gitignored), `-compare`. Scripted LLM mode feeds the heuristic's proposal through the real `LLMChooser` and its `prompt_dates` check. Live mode needs `PLANTRY_EVAL_LIVE=1`, `PLANTRY_AI_PROVIDER` (`anthropic` or `openai`), `PLANTRY_AI_API_KEY` and a model (`-model` or `PLANTRY_AI_MODEL`). The command prints a summary, writes one JSONL record per run and exits non-zero when a run failed an expectation (the report is still written). `cmd/plantry-eval/testdata/compare.golden` is rewritten with `UPDATE_GOLDEN=1 go test ./cmd/plantry-eval/...`. Re-run the suite after changing the heuristic, the checker or a prompt, and `-compare` against the previous report.

## Shopping list

Package `domain/shopping`; `GET /api/shopping-list` returns sections in walk order. Rules:

- Confirmed rows only; `include_drafts=true` adds draft sessions and draft leaf plates, and `has_drafts` reports whether the range has any either way.
- It buys the cook sessions' `portions` (not what plates consume) plus leaf components (amount × eaters), for the range's dates. The default range is `ShoppingDefault` of the anchored window; `from`/`to` override it inside the window containing `from`.
- Amounts sum in metric base units (`units.ToBase`); `display` is in the contributions' unit when they share one, otherwise grams; `buy` is set for count units only.
- `used_in` lists the sessions or leaves an item is bought for, in cell order.
- Sections are the `food.ShopSections` enum (`domain/food/shop_section.go`). A food's `shop_section` column (migration 00031) overrides `shopping.Classify`, a 3-stage data-driven classifier (packaging, phrases, head noun). `PUT /api/foods/{id}/shop-section` sets it for leaves only; errors `error.food.invalid_shop_section`, `error.food.shop_section_leaf_only`.
- `food.LoadTrees` loads recipe trees in one query per level.

## Plate audit trail

Go migration `00030_plate_events.go` (sqlc reads `schema_patches/00030_plate_events.sql`), `domain/plate/events.go`, repo `adapters/sqlite/plate_event_repo.go` (D20). 00030 also rebuilt `plates` and `cook_sessions` with `AUTOINCREMENT`, so an id never names two rows over time.

Every `plate.Service` mutation writes its events last, in its own unit of work:
- One row per plate it touches: `create` / `restore` (`before_json` null), `update`, or `delete` (`after_json` null), with full plate snapshots in the `SnapshotPlate` shape (its own `plate_id`, sessions by id). `Apply` logs `restore` when `ApplyOptions.Restore` is set (`POST /api/plan/restore`), else `create`.
- One row with `plate_id` NULL per cook session whose row changed (`{id, food_id, cook_date, …}`): fresh and snapshot sessions (`create`), D11 resizes and D13 cook-cell moves (`update`), sessions left without links (`delete`). A tracked session D11 left unchanged writes nothing.
- The explicit session edits (`SetSessionPortions`, `SetCooked`, `AddLeftovers`, `DeleteSession`) always write their session's row and set `cook_session_id` on their plate rows; `SetSessionPortions`/`SetCooked` also log every linked plate.

No FKs, so rows outlive what they describe; there is no read API yet.

The actor comes from `domain/actor` in ctx: the HTTP middleware `middleware.Actor` sets `user`, `agent.ToolSet.Execute` sets `agent` with the run's `ai_run_id`, and `actor.From` falls back to `user`.

## Setup

Package `domain/setup`. `GET /api/setup` returns `{completed, recipe_count, recipe_target}` (`setup.completed` and the count of composed foods against `setup.RecipeTarget` = 10); `POST /api/setup/complete` sets `setup.completed` and returns the same. Migration 00023 marks existing installs (any time slots) as completed.

## AI run log

Migrations `00021_ai_runs.sql` and `00026_ai_trace.sql`, package `domain/airun`, repo `adapters/sqlite/ai_run_repo.go`. Every LLM call goes through the metered client (`Recorder.Meter`, wrapped around each client in the `llmFactory` closure in `main.go`), so nothing reaches a provider unrecorded.

- **`ai_runs`** — one row per user-visible AI action.
  - `kind`: `chat`, `lookup` (food-lookup translate and pick-best), `import_fallback`; `generate`, `regenerate`, `discovery`, `classify` are reserved for later phases. `untracked` marks a metered call made without a started run — a call site that forgot `Recorder.Start`.
  - `status`: `running` until finished, then `ok` / `error` / `iterations_exceeded` / `cancelled` / `partial`. `error_key` holds the message key on failure.
  - `provider`, `model`, `prompt_version` (the `PromptVersion` const of the agent, `food`, `importer` or `planning` package — bump it on every prompt change; heuristic generations record `planning.HeuristicVersion`), `prompt_hash` → `ai_prompts`.
  - Token totals summed over all calls: `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`; `iterations`.
  - `window_start`/`window_end`/`instruction` are set for chat. Generation runs set `chooser` (`llm`/`heuristic`) and `scope_json` through `Start`, and `repair_attempts`, `violations_json`, `score_json` and `notes` (00036) through `Run.FinishWith(…, Result)`; chat runs leave them NULL. `judge_json` is unused (D20).
  - `conversation_id` (chat runs; no FK, indexed) and `request_id` (chi `middleware.GetReqID` of the HTTP request that started the run; NULL outside a request).
- **`ai_run_steps`** — one row per LLM call (`type = 'llm_call'`, `name` = model, per-call tokens) or agent tool call (`type = 'tool_call'`, `name` = tool, `args_json` and `result_json` each capped at 8 KB, `is_error`), ordered by `seq`, with `duration_ms`. LLM calls also store:
  - `request_json` = `{message_offset, messages, temperature, max_tokens, system_hash, tools_hash, truncated}`. `messages` is `req.Messages[message_offset:]`: the offset is the message count of the run's previous LLM call when this call resends those messages unchanged, else 0 (also 0 after a step that failed to store). `system_hash` → `ai_prompts`, `tools_hash` → `ai_tool_schemas` (sha256 of the JSON of `req.Tools`), each null when absent.
  - `response_json` = `{content, stop_reason}` (the reply's text and tool_use blocks) and `stop_reason`; both NULL when the call failed (`is_error = 1`).
  - Each body is capped at 256 KiB: the largest text / tool payloads are shortened (ending in `…[truncated]`) so it stays valid JSON, and `truncated` is set.
- **`ai_prompts`** — system prompt text stored once per distinct hash. Every distinct system prompt of a run is stored (a lookup has two: translate and pick-best); `ai_runs.prompt_hash` is the first call's.
- **`ai_tool_schemas`** — tool definitions (`json`, a `[]llm.Tool`) stored once per distinct hash.
- **Replay.** Call *i*'s request is call *i−1*'s messages cut to `message_offset` plus call *i*'s `messages`, the prompt by `system_hash`, the tools by `tools_hash`, `temperature`, `max_tokens` and the step's `name` as model. Callers append the assistant reply to their history themselves, so replies are already in the next delta; never insert `response_json` into a reconstruction. `domain/airun/replay_test.go` (`reconstruct`) is the reference.
- **Retention.** At startup and after a finished run (at most once per hour; both in a goroutine, deleting in batches of 500; `Recorder.Wait` waits for them and stops new ones from starting) the steps of runs started more than `ai.run_retention_days` days ago (setting, default 180) are deleted. `ai_runs`, `ai_prompts` and `ai_tool_schemas` rows stay. There is no background scheduler; prune failures are logged at warn and not retried for an hour.
- **Cost is not stored.** `airun.Cost(model, provider, tokens, overrides)` (`cost.go`) computes it on read: an `ai.price_overrides` entry for the exact model id wins, else the built-in table entry with the longest model-id prefix (`PricesAsOf`, sources cited next to the table). Missing cache prices default to 0.1× / 1.25× of the input price (0.5× / 0 for OpenAI, whose input count includes the cached tokens, which are subtracted). An unpriced model is `nil` ("cost unknown"); a run without tokens costs 0. slog also emits `ai.run.start` / `ai.run.end` with `run_id`.
- **Read API** (`handlers/ai_runs.go`, `airun.Insights`, repo `ai_run_insights_repo.go`; one query per aggregate):
  - `GET /api/ai/runs?kind&from&to&cursor&limit` (50, max 200) — newest first, `next_cursor` is the last id of the page; items carry `tokens`, `cost_estimate_usd`, `score`, `outcome`, `correction_rate`.
  - `GET /api/ai/runs/{id}` — plus `scope`, `instruction`, `notes`, `violations`, `error_key`, `prompt_text` and `steps` in seq order; a step's `args`/`result` are text cut to 8192 characters (`*_truncated`), an llm_call's are its request/response JSON.
  - `GET /api/ai/insights?from&to&bucket=week` (default: the last 12 weeks) — Monday weeks with `runs`, `accepted` (incl. partially), `discarded`, `correction_rate`, `cost_usd` (priced runs), `cost_per_accepted`, `unpriced_runs`; `by_version` groups by kind, prompt version and model with `score_avg` (numeric leaves of `score_json`, dotted keys) and violation counts by rule.
  - `GET /api/ai/prices?model=` (default: the configured model) — `{model, price|null, source: builtin|override|none, as_of}`.
  - Correction rate (D24, sqlc `AIRunCorrections`): plates a run's `generator` `create` events made, and how many of them got a `user` `update`/`delete` event before the plate's first `accept`/`confirm` event.
- **Tool choice.** `llm.Request.ToolChoice` forces a tool: Anthropic sends `tool_choice: {type: tool, name}`, OpenAI `{type: function, function: {name}}`; both send `temperature` only when it is > 0. The metered client stores it as `tool_choice` in `request_json`. The fake client's script already replays scripted `tool_use` turns.
- **LLM chooser** (`planning/llmchooser.go`, prompt `planning/prompt.go`, D11–D14): one forced `submit_plan` call per attempt (temperature 0.4, max_tokens 16000) under the run's ctx, so each call is a step; a proposal with violations or an unreadable reply goes back as a tool result, at most twice (`repair_attempts`, stage `repairing`). Violations left after that are persisted (`partial`); a provider failure ends the run `error.ai.provider_error`, no readable reply at all `error.ai.invalid_plan`. A request without `chooser` uses the LLM only when a provider resolves. The prompt keeps within 40 k estimated tokens (chars / 4) by leaving out candidates. E2E fixtures: `frontend/e2e/fixtures/chat-scripts/generate-week.json` (a template; placeholders in its `_comment`) and `generate-week-error.json`.

Query it with the `sqlite3` CLI if installed, otherwise Python's stdlib `sqlite3` module (the path is `PLANTRY_DB_PATH`):

```bash
sqlite3 /data/plantry.db "SELECT id, kind, status, model, input_tokens, output_tokens, started_at FROM ai_runs ORDER BY id DESC LIMIT 20"

python3 -c 'import sqlite3,sys; c=sqlite3.connect(sys.argv[1]); [print(r) for r in c.execute("SELECT id, kind, status, model, input_tokens, output_tokens, started_at FROM ai_runs ORDER BY id DESC LIMIT 20")]' /data/plantry.db

# steps of one run
sqlite3 /data/plantry.db "SELECT seq, type, name, is_error, duration_ms, input_tokens, output_tokens FROM ai_run_steps WHERE run_id = 42 ORDER BY seq"

# what call 1 of run 42 sent and got back
sqlite3 /data/plantry.db "SELECT request_json, response_json FROM ai_run_steps WHERE run_id = 42 AND type = 'llm_call' ORDER BY seq LIMIT 1"
```

## Key Dependencies

| Package | Purpose |
|---|---|
| `go-chi/chi/v5` | HTTP router |
| `modernc.org/sqlite` | CGO-free SQLite driver |
| `pressly/goose/v3` | DB migrations |
| `Masterminds/squirrel` | SQL query builder (dynamic queries) |
| `xeipuuv/gojsonschema` | JSON Schema validation (AI responses) |

## Coding Rules

- **`errors.Is()`, never `==`** for sentinel errors. Wrapped errors break `==` silently.
- **Domain validation in the service, not the DB.** DB constraints are a safety net only.
- **Sanitize FTS5 input.** Pass user search strings through `sanitizeFTS5()` before MATCH.
- **Use sqlc-generated functions, not raw SQL.** Change query annotations and regenerate if needed.
- **`testhelper.NewTestDB()` must mirror production config** (same pragmas, `SetMaxOpenConns(1)`, etc).

## Testing

**Unit tests:** pure, no DB, no HTTP. Use fakes from `adapters/fake/`.

**Adapter tests:** real SQLite via `testhelper.NewTestDB()`. Never mock the database.

**Migration tests:** `migration_000XX_test.go` in `adapters/sqlite/` — test data shape before and after.

### Coverage Requirements

Every feature must cover happy path **and** edge cases before it's complete:

- Validation: empty/missing required fields, invalid enums, out-of-range numbers, boundary values
- Conflicts: duplicate names, update/delete non-existent, in-use references
- Input sanitization: FTS operators, SQL-sensitive chars, unicode
- Nullable roundtrips: create with value → update to null → verify null
- HTTP edge cases: malformed JSON, non-numeric IDs, invalid query params

## Worktrees

Inside `.claude/worktrees/*`, prefix Go commands with `GOWORK=off` (e.g. `GOWORK=off go test ./...`). The root `go.work` created by `mise run lsp` lists only the main checkout's `./backend`, so without it Go fails with "directory prefix . does not contain modules listed in go.work".

## SQLite Gotchas

- Pragmas are per-connection. `sql.DB` pools — use `SetMaxOpenConns(1)` so all queries share WAL + `busy_timeout`.
- Timestamps: stored as `TEXT` via `datetime('now')`, Go parse layout `"2006-01-02 15:04:05"`, serialize as RFC3339 in API responses.
