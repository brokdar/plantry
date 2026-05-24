---
name: hexagonal-architecture
description: >
  Design and implement features in Plantry's hexagonal-lite backend. Use this
  skill whenever adding a new aggregate, refactoring a service, wiring a new
  adapter, defining a repository interface, or writing tests for any backend
  layer. Load it for any task touching backend/internal/domain, backend/internal/adapters,
  backend/internal/transport, or backend/cmd/plantry/main.go.
---

# Hexagonal Architecture — Plantry Backend

Plantry uses a hexagonal-lite architecture sometimes described as
`transport → domain ← adapters`. Business logic lives in `domain`; it depends
on nothing external. Adapters and transport surround it and depend inward.

## Layer Map

```
cmd/plantry/main.go          ← composition root: wires everything once
internal/
  transport/http/             ← inbound layer  (HTTP handlers, chi routing)
  domain/<aggregate>/         ← pure business logic (no I/O, no framework)
  adapters/sqlite/            ← outbound layer (SQLite via sqlc)
  adapters/<other>/           ← other outbound adapters (HTTP clients, AI, crypto…)
```

Dependency direction is always inward:

```
transport → domain ← adapters
```

`domain` may only import other `domain` sub-packages and the stdlib. It must
never import `transport`, `adapters`, `net/http`, or any sqlc-generated type.

---

## Layer Responsibilities

### `domain/<aggregate>/`

Each aggregate owns four files:

| File | Purpose |
|---|---|
| `<entity>.go` | Plain exported struct. No methods that touch I/O. |
| `repository.go` | Port interface the adapter must satisfy. |
| `service.go` | Business logic. Constructor takes the interface. |
| `service_test.go` | Pure unit tests using an in-file fake. |

Rules:
- `Service` holds the repo interface as a field, never a concrete type.
- Validation lives in the service, not the DB.
- Use `domain.Err*` sentinel errors from `internal/domain/errors.go`.
- Wrap sentinels with `fmt.Errorf("%w: …", domain.ErrXxx)` to add context.
- Use `errors.Is()`, never `==`, to check sentinel errors.

**Example — `slot/repository.go`:**
```go
package slot

import "context"

type Repository interface {
    Create(ctx context.Context, s *TimeSlot) error
    Get(ctx context.Context, id int64) (*TimeSlot, error)
    Update(ctx context.Context, s *TimeSlot) error
    Delete(ctx context.Context, id int64) error
    List(ctx context.Context, activeOnly bool) ([]TimeSlot, error)
    CountPlatesUsing(ctx context.Context, slotID int64) (int64, error)
}
```

**Example — `slot/service.go`:**
```go
type Service struct {
    repo Repository
}

func NewService(repo Repository) *Service {
    return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, t *TimeSlot) error {
    if t.NameKey == "" {
        return fmt.Errorf("%w: name_key required", domain.ErrInvalidInput)
    }
    return s.repo.Create(ctx, t)
}
```

---

### `adapters/sqlite/`

Each aggregate gets a `<aggregate>_repo.go` that implements its domain
`Repository` interface backed by sqlc-generated functions.

**sqlc workflow:**
1. Write or edit `adapters/sqlite/queries/<aggregate>.sql` with `-- name: X :one` annotations.
2. Run `sqlc generate` from `backend/`.
3. Generated types and functions land in `adapters/sqlite/sqlcgen/` — never edit by hand.
4. Implement `<aggregate>_repo.go` using the generated functions.

The generated `sqlcgen.*` types are internal to the adapter. Map them to domain
structs with a private `mapXxxToDomain` function. Domain code must never see
sqlcgen types.

**Example — `slot_repo.go`:**
```go
type SlotRepo struct {
    db *sql.DB
    q  *sqlcgen.Queries
}

func NewSlotRepo(db *sql.DB) *SlotRepo {
    return &SlotRepo{db: db, q: sqlcgen.New(db)}
}

func (r *SlotRepo) Get(ctx context.Context, id int64) (*slot.TimeSlot, error) {
    row, err := r.q.GetTimeSlot(ctx, id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
        }
        return nil, err
    }
    var s slot.TimeSlot
    mapTimeSlotToDomain(&row, &s)
    return &s, nil
}

func mapTimeSlotToDomain(row *sqlcgen.TimeSlot, s *slot.TimeSlot) {
    s.ID = row.ID
    s.NameKey = row.NameKey
    // …
}
```

**Cross-compilation constraint:** No adapter may introduce CGO. This project
cross-compiles to ARM (linux/arm64) with CGO disabled. Use `modernc.org/sqlite`
(pure Go). Never use `mattn/go-sqlite3` or any CGO-dependent package.

---

### `transport/http/handlers/`

One file per aggregate: `<aggregate>.go`. Handler structs take domain service
interfaces, not concrete types (though in practice the `*Service` concrete type
is often used directly since there is only one implementation — prefer interface
params if you need test mocking without a real DB).

Rules:
- Parse request → call domain service → encode response. Nothing else.
- Translate domain errors to HTTP status codes using `toHTTPWithResource` from
  `errormap.go`. Never construct custom HTTP error codes inline.
- Return `application/json` on all responses including errors.
- Route registration lives in `transport/http/router.go`, not in handlers.

**Error translation pattern:**
```go
func (h *SlotHandler) Delete(w http.ResponseWriter, r *http.Request) {
    id := parseID(r)
    if err := h.svc.Delete(r.Context(), id); err != nil {
        status, code := toHTTP(err)
        writeError(w, status, code)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

Domain errors bubble up from service → handler; `toHTTPWithResource` centralises
the mapping so each handler doesn't re-implement it.

---

### `cmd/plantry/main.go` — Composition Root

All wiring happens once, here. The pattern:

1. Open DB → run migrations.
2. Construct adapters (`sqlite.NewXxxRepo(conn)`).
3. Construct domain services (`xxx.NewService(repo)`).
4. Construct handlers (`handlers.NewXxxHandler(svc)`).
5. Register routes (`transport.NewRouter(…, h)`).

Never wire dependencies inside domain or adapter packages. If a domain service
needs another service, pass it as a constructor argument from `main.go`.

---

## Test Strategy Per Layer

### Domain tests (`domain/<aggregate>/service_test.go`)

- Pure Go. No DB. No HTTP.
- Define a `fakeRepo` struct in the test file that implements the `Repository`
  interface using an in-memory map.
- Test validation, business rules, and sentinel error propagation.

```go
type fakeRepo struct {
    items  map[int64]*slot.TimeSlot
    nextID int64
}

func (r *fakeRepo) Create(_ context.Context, t *slot.TimeSlot) error {
    t.ID = r.nextID; r.nextID++
    cp := *t; r.items[t.ID] = &cp
    return nil
}
// … implement all interface methods …

func TestService_Create_Validation(t *testing.T) {
    svc := slot.NewService(newFakeRepo())
    err := svc.Create(context.Background(), &slot.TimeSlot{NameKey: ""})
    require.True(t, errors.Is(err, domain.ErrInvalidInput))
}
```

### Adapter tests (`adapters/sqlite/<aggregate>_repo_test.go`)

- Hit a real in-memory SQLite via `testhelper.NewTestDB(t)`. Never mock the DB.
- `NewTestDB` creates a temp file DB, runs all goose migrations, and registers
  cleanup automatically — just call it and use the `*sql.DB`.
- Test full CRUD round-trips, `ErrNotFound` on missing IDs, `ErrInUse` on FK
  violations, and nullable column round-trips.

```go
func TestSlotRepo_RoundTrip(t *testing.T) {
    repo := sqlite.NewSlotRepo(testhelper.NewTestDB(t))
    ctx := context.Background()

    s := &slot.TimeSlot{NameKey: "slot.breakfast", Icon: "Coffee", Active: true}
    require.NoError(t, repo.Create(ctx, s))
    assert.NotZero(t, s.ID)

    got, err := repo.Get(ctx, s.ID)
    require.NoError(t, err)
    assert.Equal(t, "slot.breakfast", got.NameKey)

    require.NoError(t, repo.Delete(ctx, s.ID))
    _, err = repo.Get(ctx, s.ID)
    assert.True(t, errors.Is(err, domain.ErrNotFound))
}
```

### Transport tests (`transport/http/handlers/<aggregate>_test.go`)

- Use `net/http/httptest`. Build a mini chi router wired to real adapters via
  `testhelper.NewTestDB(t)`. This is the Plantry convention: handler tests use
  a real SQLite so they cover the full stack from HTTP parsing to DB and back.
- Alternatively, if the feature has expensive or non-deterministic external
  dependencies, inject a mock service struct that implements the domain
  interface.

```go
func setupSlotRouter(t *testing.T) http.Handler {
    t.Helper()
    db := testhelper.NewTestDB(t)
    svc := slot.NewService(sqlite.NewSlotRepo(db))
    h := handlers.NewSlotHandler(svc)
    r := chi.NewRouter()
    r.Route("/api/settings/slots", func(r chi.Router) {
        r.Get("/", h.List)
        r.Post("/", h.Create)
        r.Put("/{id}", h.Update)
        r.Delete("/{id}", h.Delete)
    })
    return r
}

func TestSlotHandler_Create_MissingNameKey(t *testing.T) {
    r := setupSlotRouter(t)
    resp := httptest.NewRecorder()
    r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/settings/slots",
        bytes.NewBufferString(`{"icon":"Coffee"}`)))
    assert.Equal(t, http.StatusBadRequest, resp.Code)
}
```

---

## Adding a New Feature — Checklist

1. **Domain model** — add/edit `domain/<aggregate>/<entity>.go`.
2. **Port** — declare the `Repository` interface in `domain/<aggregate>/repository.go`.
3. **Service** — implement business logic in `domain/<aggregate>/service.go`.
4. **Domain tests** — write `service_test.go` with a fake repo covering happy path and all error cases.
5. **SQL queries** — write `adapters/sqlite/queries/<aggregate>.sql`, run `sqlc generate`.
6. **Adapter** — implement `adapters/sqlite/<aggregate>_repo.go` using sqlcgen functions; map sqlcgen types to domain types.
7. **Adapter tests** — write `<aggregate>_repo_test.go` using `testhelper.NewTestDB(t)`.
8. **Handler** — add/edit `transport/http/handlers/<aggregate>.go`; use `toHTTPWithResource` for error translation.
9. **Route** — register routes in `transport/http/router.go` under the `Handlers` struct.
10. **Wire** — instantiate adapter → service → handler in `cmd/plantry/main.go`.
11. **Handler tests** — write `<aggregate>_test.go` with httptest covering HTTP status codes, validation, and not-found paths.

---

## Anti-Patterns to Avoid

- Domain structs importing sqlcgen types, `net/http`, or chi. If you see this, stop and redesign.
- Raw SQL strings outside `adapters/sqlite/queries/*.sql` files. Use sqlc.
- Parsing `*http.Request` inside a domain service.
- Returning sqlcgen row types from repo methods. Always map to domain types.
- Wiring dependencies outside `main.go` (hidden globals, `init()` side-effects, `sync.Once` service locators).
- Using `mattn/go-sqlite3` or any CGO package — the binary must cross-compile to ARM without CGO.
- Checking errors with `==` instead of `errors.Is()`.
- Adding new sentinel errors in transport or adapters packages; all shared sentinels live in `internal/domain/errors.go`.

---

## SQLite Specifics

- Pragmas are per-connection. Production uses `SetMaxOpenConns(1)`. Replicate in tests via `testhelper.NewTestDB`.
- Timestamps stored as `TEXT` (`datetime('now')`). Parse layout: `"2006-01-02 15:04:05"`. Serialize as RFC3339 in JSON responses.
- FTS5 input must be sanitised through `sanitizeFTS5()` before use in `MATCH` queries.
- Dynamic queries use `Masterminds/squirrel`; static queries use sqlc.
- Never write raw `database/sql` queries inline in repo files — put them in `.sql` files and regenerate.

---

## Quick Reference

| Question | Answer |
|---|---|
| Where does business logic live? | `domain/<aggregate>/service.go` |
| Where do I add a new DB query? | `adapters/sqlite/queries/<aggregate>.sql` then `sqlc generate` |
| Where do I add a new HTTP route? | `transport/http/router.go` (register) + `handlers/<aggregate>.go` (implement) |
| Where do I wire a new service? | `cmd/plantry/main.go` only |
| Which SQLite driver? | `modernc.org/sqlite` (pure Go, CGO-free) |
| Which error check idiom? | `errors.Is(err, domain.ErrXxx)` |
| How do I test a repo? | `testhelper.NewTestDB(t)` + real SQL |
| How do I test a service? | Inline `fakeRepo` struct, no DB |
| How do I test a handler? | `httptest` + chi mini-router + `testhelper.NewTestDB(t)` |
