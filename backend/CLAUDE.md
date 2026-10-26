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
```

## Required Skills

Load these before writing or modifying backend code:

- **`golang-patterns`** — Go code
- **`golang-testing`** — Go tests

## Architecture — Hexagonal-Lite

Data flow: `transport → domain ← adapters`

```
backend/
├── cmd/plantry/main.go              # entrypoint — wires everything together
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
    │   ├── plate/                   # Plate aggregate (meals)
    │   ├── slot/                    # Slot aggregate (planner day slots)
    │   ├── preset/                  # Preset aggregate (meal templates)
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
| `plan.week_starts_on` | `monday` | `monday` / `sunday` / `saturday` |
| `plan.anchor` | `next_shopping_day` | `today` / `next_shopping_day` / `fixed_weekday` |
| `plan.shopping_day` | `0` | 0 = Monday … 6 = Sunday |
| `setup.completed` | `false` | Set by `POST /api/setup/complete` |
| `plan.max_cook_occasions_per_day`, `plan.batch_cooking`, `plan.default_keep_days`, `plan.freezing_enabled`, `plan.freezer_keep_days`, `plan.variety_weeks`, `plan.explore_per_week`, `discovery.daily_enabled` | see `specs.go` | Stored and edited in Settings; read by later phases |
| `ai.run_retention_days` | `180` | 7–3650; see AI run log |

The profile (`domain/profile`) also carries `diet_type`, `meat_days_per_week_max` and `household_portions` (00023).

## Availability

Migration `00024_availability.sql`, package `domain/availability`. A cell is (date, slot) and its mode is `plan`, `routine` (needs a `preset_id`; a label only for now, nothing applies the preset) or `none` (No plate). It resolves as per-date override → weekday default (weekday 0 = Monday) → `plan`. `plan` defaults are never stored.

- `GET /api/availability?from=&to=` — resolved cells with `source` (`override` / `default` / `fallback`), at most 62 days.
- `GET` / `PUT /api/availability/defaults` — `{items: [{weekday, slot_id, mode, preset_id}]}`; PUT replaces the defaults of all active slots.
- `PUT /api/availability/{date}/{slotId}` — `{mode, preset_id?, note?, remove_plate?}`. `none` on a cell with a plate is `409 error.availability.cell_has_plate` unless `remove_plate` is set; then the plate is deleted in the same transaction and returned as `removed_plate`. `DELETE` clears the override.

A cell holds at most one plate: migration `00025_plates_cell_unique.go` turned the old per-plate skip flag into `none` overrides (with its note), merged duplicates, dropped the flag column and made `(date, slot_id)` unique. Creating or moving a plate into a `none` cell is `409 error.plate.cell_unavailable`; into an occupied cell, `409 error.plate.cell_occupied`. The agent sees `none` and `routine` cells and must not plan them.

## Setup

Package `domain/setup`. `GET /api/setup` returns `{completed, recipe_count, recipe_target}` (`setup.completed` and the count of composed foods against `setup.RecipeTarget` = 10); `POST /api/setup/complete` sets `setup.completed` and returns the same. Migration 00023 marks existing installs (any time slots) as completed.

## AI run log

Migrations `00021_ai_runs.sql` and `00026_ai_trace.sql`, package `domain/airun`, repo `adapters/sqlite/ai_run_repo.go`. Every LLM call goes through the metered client (`Recorder.Meter`, wrapped around each client in the `llmFactory` closure in `main.go`), so nothing reaches a provider unrecorded.

- **`ai_runs`** — one row per user-visible AI action.
  - `kind`: `chat`, `lookup` (food-lookup translate and pick-best), `import_fallback`; `generate`, `regenerate`, `discovery`, `classify` are reserved for later phases. `untracked` marks a metered call made without a started run — a call site that forgot `Recorder.Start`.
  - `status`: `running` until finished, then `ok` / `error` / `iterations_exceeded` / `cancelled` / `partial`. `error_key` holds the message key on failure.
  - `provider`, `model`, `prompt_version` (the `PromptVersion` const of the agent, `food` or `importer` package — bump it on every prompt change), `prompt_hash` → `ai_prompts`.
  - Token totals summed over all calls: `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`; `iterations`.
  - `window_start`/`window_end`/`instruction` are set for chat; the scoring/outcome columns (`scope_json`, `chooser`, `violations_json`, `score_json`, `judge_json`, `outcome*`, `repair_attempts`) are reserved.
  - `conversation_id` (chat runs; no FK, indexed) and `request_id` (chi `middleware.GetReqID` of the HTTP request that started the run; NULL outside a request).
- **`ai_run_steps`** — one row per LLM call (`type = 'llm_call'`, `name` = model, per-call tokens) or agent tool call (`type = 'tool_call'`, `name` = tool, `args_json` and `result_json` each capped at 8 KB, `is_error`), ordered by `seq`, with `duration_ms`. LLM calls also store:
  - `request_json` = `{message_offset, messages, temperature, max_tokens, system_hash, tools_hash, truncated}`. `messages` is `req.Messages[message_offset:]`: the offset is the message count of the run's previous LLM call when this call resends those messages unchanged, else 0 (also 0 after a step that failed to store). `system_hash` → `ai_prompts`, `tools_hash` → `ai_tool_schemas` (sha256 of the JSON of `req.Tools`), each null when absent.
  - `response_json` = `{content, stop_reason}` (the reply's text and tool_use blocks) and `stop_reason`; both NULL when the call failed (`is_error = 1`).
  - Each body is capped at 256 KiB: the largest text / tool payloads are shortened (ending in `…[truncated]`) so it stays valid JSON, and `truncated` is set.
- **`ai_prompts`** — system prompt text stored once per distinct hash. Every distinct system prompt of a run is stored (a lookup has two: translate and pick-best); `ai_runs.prompt_hash` is the first call's.
- **`ai_tool_schemas`** — tool definitions (`json`, a `[]llm.Tool`) stored once per distinct hash.
- **Replay.** Call *i*'s request is call *i−1*'s messages cut to `message_offset` plus call *i*'s `messages`, the prompt by `system_hash`, the tools by `tools_hash`, `temperature`, `max_tokens` and the step's `name` as model. Callers append the assistant reply to their history themselves, so replies are already in the next delta; never insert `response_json` into a reconstruction. `domain/airun/replay_test.go` (`reconstruct`) is the reference.
- **Retention.** At startup and after a finished run (at most once per hour; both in a goroutine, deleting in batches of 500; `Recorder.Wait` waits for them and stops new ones from starting) the steps of runs started more than `ai.run_retention_days` days ago (setting, default 180) are deleted. `ai_runs`, `ai_prompts` and `ai_tool_schemas` rows stay. There is no background scheduler; prune failures are logged at warn and not retried for an hour.
- **Cost is not stored.** It is computed from tokens on read (Phase 3). There is no runs API yet; slog also emits `ai.run.start` / `ai.run.end` with `run_id`.

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
