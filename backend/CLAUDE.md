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
    │   ├── agent/                   # AI agent aggregate
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
| `SECRET_KEY` | — | 32-char hex; enables DB encryption for stored API keys |
| `DEV_MODE` | `false` | Exposes debug-only endpoints |

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

## SQLite Gotchas

- Pragmas are per-connection. `sql.DB` pools — use `SetMaxOpenConns(1)` so all queries share WAL + `busy_timeout`.
- Timestamps: stored as `TEXT` via `datetime('now')`, Go parse layout `"2006-01-02 15:04:05"`, serialize as RFC3339 in API responses.
