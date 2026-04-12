# Plantry

Self-hosted weekly meal planner. Single Go binary embeds SPA, serves from one SQLite file. Must cross-compile to ARM (no CGO).

## Commands

### Backend

```bash
cd backend
go build ./...                        # compile
go test ./...                         # all tests
go test -run TestName ./path/...      # single test
go vet ./...                          # vet
golangci-lint run                     # lint
sqlc generate                         # regenerate from queries + migrations
go test -race ./...                   # race detector — run before feature complete
```

### Frontend

```bash
cd frontend
bun install                           # install deps
bun run dev                           # dev server (port 5173, proxies /api → :8080)
bun run build                         # production build → dist/
bun run typecheck                     # tsc --noEmit
bun run lint                          # eslint
bun run test                          # vitest (unit)
bun run e2e                           # playwright (needs running backend)
bun run check                         # lint + typecheck + unit tests
```

### Docker

```bash
docker compose up --build             # build + run (port 8080)
```

## Required Skills

Load these before writing or modifying the relevant code:

- **`frontend-design`** - creating/modifying frontend pages/components
- **`vitest`** — frontend unit tests
- **`playwright-best-practices`** — e2e tests
- **`vercel-react-best-practices`** — React components
- **`vercel-composition-patterns`** — component API design / refactoring prop interfaces
- **`golang-testing`** — Go tests
- **`golang-patterns`** — Go code

## Architecture

### Backend — Hexagonal-Lite (`transport → domain ← adapters`)

```
backend/internal/
├── adapters/sqlite/   # sqlc queries (queries/ + sqlcgen/) + goose migrations
├── domain/            # aggregates, services, repo interfaces
├── transport/http/
│   ├── router.go      # chi route registrations
│   └── handlers/      # thin HTTP translation, one file per aggregate
├── webui/             # embeds frontend dist/
├── config/            # env var loading
└── testhelper/        # NewTestDB, HTTP fixtures
```

Aggregates are plain exported structs. `*.Service` holds business logic, takes repo interfaces as constructor args.

### Frontend — TanStack Router (file-based)

```
frontend/src/
├── routes/            # file-based (auto-generates routeTree.gen.ts — never edit)
├── components/ui/     # shadcn primitives
├── lib/
│   ├── api/           # fetch wrappers per resource
│   ├── queries/       # TanStack Query hooks + key factories
│   ├── schemas/       # zod validation schemas
│   └── i18n/          # i18next (react-i18next)
└── test/              # render.tsx (renderWithRouter), fixtures.ts (mock data)
```

Forms: `react-hook-form` + `zod`. Design tokens: `frontend/src/index.css` (OKLch).

## Coding Rules

- **`errors.Is()`, never `==`** for sentinel errors. Wrapped errors break `==` silently.
- **Domain validation in the service, not the DB.** DB constraints are a safety net only.
- **Sanitize FTS5 input.** Pass user search strings through `sanitizeFTS5()` before MATCH.
- **Use sqlc-generated functions, not raw SQL.** Change query annotations and regenerate if needed.
- **`testhelper.NewTestDB()` must mirror production config** (pragmas, `SetMaxOpenConns(1)`, etc).

## Testing

### Strategy

- **Unit tests:** pure, no DB, no HTTP.
- **Adapter tests:** real SQLite via `testhelper.NewTestDB()`. Never mock the database.
- **e2e:** Playwright, requires both backend and frontend running.

### Coverage Requirements

Every feature must cover happy path **and** edge cases across all layers before it's complete:

- Validation: empty/missing required fields, invalid enums, out-of-range numbers, boundary values
- Conflicts: duplicate names, update/delete non-existent, in-use references
- Input sanitization: FTS operators, SQL-sensitive chars, unicode
- Nullable roundtrips: create with value → update to null → verify null
- HTTP edge cases: malformed JSON, non-numeric IDs, invalid query params

### Frontend Vitest

- Mock at `@/lib/api/*` module level — TanStack Query works normally with mocked fetch fns.
- Use shared `renderWithRouter` from `@/test/render` — wraps in `QueryClientProvider` (retry: false) + `RouterProvider` (createMemoryHistory).
- Use shared fixtures from `@/test/fixtures` — don't duplicate mock data per test file.
- Use `screen.findBy*` (async) — `RouterProvider` renders async.

### Playwright E2E

- Self-contained: seed via direct backend API (`http://localhost:8080`), cleanup in `finally`. Always bypass Vite proxy for seeding.
- Unique data: append `crypto.randomUUID().slice(0,8)` to names.
- After form submit: `waitForResponse` on the specific API call. Never `waitForURL`, never `waitForTimeout`.
- For elements absent from DOM (server-filtered): use `toHaveCount(0)`, never `not.toBeVisible()` (causes timeouts).
- Stable = `--repeat-each=10 --workers=4` with zero failures.

## Gotchas

### SQLite

- Pragmas are per-connection. `sql.DB` pools — use `SetMaxOpenConns(1)` so all queries share WAL + busy_timeout.
- Timestamps: `TEXT` via `datetime('now')`, Go layout `"2006-01-02 15:04:05"`, serialize as RFC3339 in API.

### React / ESLint

- **No ref access during render** (`react-hooks/refs`). The "compare prev ref value" pattern fails lint. Reset derived state in event handlers instead.
- **No setState in effects** (`react-hooks/set-state-in-effect`). Move state resets to event handlers.
- **Prefer `useDeferredValue`** over `useState`+`useEffect` debounce.

### shadcn

- `FormControl` uses `Slot.Root` (radix-ui) instead of `<span>` so id/aria forward to the child input. `useFormField` reads id from `FormItemContext`.
