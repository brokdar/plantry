# Plantry

**A self-hosted weekly meal planner for home cooks who care about what they eat.**

Plantry runs entirely on your own hardware — a Raspberry Pi, NAS, or laptop — in a single ~30 MB Docker container. No cloud. No subscription. No data leaves your machine.

---

## The core idea

Most meal planners model a planned meal as "one recipe." That's wrong.

A real dinner is a **main + sides + sauce**. A breakfast is "oatmeal + fruit + coffee." Plantry's core data unit is the **component** (a self-contained dish), and the planning unit is the **plate** — a composition of 1–N components.

Swapping rice for naan is one click. Adding a side salad is one click. Tofu curry and chicken curry are siblings in a variant group, not duplicated recipes. This single modelling decision — **plates, not recipes** — is what makes everything else simple.

---

## Features

**Plan the week**
- A plan board of 7 days × your time slots (a day scroll on phones), shown for the window that starts on your shopping day
- Say who is home: away, plan or guests per day and slot ("No plate" cells), or save a routine (a fixed composition) and apply it to a day
- **Generate** drafts for the whole window, the empty cells or a mid-week re-plan. A deterministic heuristic works without any AI; with a provider key an LLM chooses, and every proposal passes the same hard-rule checker (diet, repeats, cook days, batch rules)
- Review the drafts, confirm single batches or **Accept and shop**; the checked plan feeds Today and the shopping list
- Batch cooking: cook once, eat the leftovers on later days, optionally freeze portions (only for recipes marked freezable)

**Assistant**
- A panel on the plan board where you ask for changes in plain words; every edit is a change line with Undo
- It only edits drafts. Confirmed plates stay put until you say yes to an unlock request
- Bring your own OpenAI or Anthropic key. Everything else works without one

**Today and Cook**
- Today shows what to cook and eat, with the day's macros against your targets, a freezer shelf and a quick way to rate a meal
- Cook mode: one step per screen, detected timers, the screen stays on

**Recipes and ingredients**
- Recipes are foods composed of ingredients; variants link siblings (tofu curry, chicken curry)
- Nutrition is computed from the ingredients (kcal, protein, fat, carbs), per portion, plate and day
- Ingredients: barcode scan and Open Food Facts, name search in USDA FoodData Central, manual entry
- Import from a URL (JSON-LD first, an LLM only as a fallback), with the ingredient review before anything is saved

**Discover new recipes**
- With an AI provider set, Plantry researches recipes you do not have yet. A daily idea appears over Today, and Recipes has a Discover sheet (a cuisine to explore, or recipes that reuse what you already buy)
- The app fetches every suggested page itself and keeps only real recipes; nutrition is computed from your ingredient catalogue, and ingredients you have never used are marked new
- "Add" opens the normal import review, pre-filled; "Not for me" teaches the next run
- Anthropic models search the web (billed per search, shown in Insights); other providers suggest from the model's knowledge and the pages are still verified

**Shopping**
- Aggregated from the confirmed plan, grouped by store section, summed in base units and rounded up to buyable amounts
- An in-store mode with big rows and the screen kept on

**Settings and insights**
- Household, meals and week, cooking, assistant and system sections; English and German
- Insights lists every AI run with its prompt version, tokens, cost and outcome

---

## Quick start

```bash
# Clone the repo
git clone https://github.com/jaltszeimer/plantry.git
cd plantry

# Start with Docker Compose (port 8080)
docker compose up --build
```

Open [http://localhost:8080](http://localhost:8080).

> [!NOTE]
> Data is persisted in a Docker volume (`plantry-data`). The SQLite database lives at `/data/plantry.db` inside the container, next to `secret.key` (see below).

---

## Configuration

All configuration is via environment variables. Set them in `docker-compose.yml` or pass them directly.

| Variable | Default | Description |
|---|---|---|
| `PLANTRY_PORT` | `8080` | HTTP server port |
| `PLANTRY_DB_PATH` | `/data/plantry.db` | SQLite database path |
| `PLANTRY_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `PLANTRY_FDC_API_KEY` | — | USDA FoodData Central API key (optional) |
| `PLANTRY_AI_PROVIDER` | — | `openai` or `anthropic` (optional) |
| `PLANTRY_AI_MODEL` | — | Model name for chosen provider |
| `PLANTRY_AI_API_KEY` | — | API key for the AI provider |
| `PLANTRY_SECRET_KEY` | auto-generated | Key that encrypts API keys saved in the UI (optional, at least 32 chars) |
| `PLANTRY_OFF_DISABLED` | `false` | Skip Open Food Facts lookups (used by the e2e tests) |

AI features are entirely optional. Plantry is fully usable without an API key.

API keys entered in Settings are stored encrypted in the database. On first start Plantry generates a random key (64 hex chars) and saves it as `secret.key` next to the database, mode 0600. Keep that file with the database: without it, stored API keys cannot be decrypted. Setting `PLANTRY_SECRET_KEY` overrides the file, and no file is written.

### Backup and restore

Copying `/data` while Plantry runs is not safe (the database is in WAL mode). Use the `backup` command instead; it takes a consistent snapshot while the server keeps running and copies `secret.key` beside it:

```bash
docker compose exec plantry plantry backup -out /data/backup/plantry-$(date +%F).db
docker compose cp plantry:/data/backup ./backup     # take it off the volume
```

(Create the target directory first, e.g. `docker compose exec plantry mkdir -p /data/backup`.) The target file must not exist yet.

To restore: stop Plantry, put the backup file at `/data/plantry.db` and its `secret.key` at `/data/secret.key` (remove any `plantry.db-wal` and `plantry.db-shm` next to the old database), then start Plantry. Pending migrations run on start, so a backup from an older version restores into a newer one.

---

## Development

### Requirements

- [Go 1.25+](https://go.dev/dl/)
- [Bun](https://bun.sh/) (frontend package manager)
- [Docker](https://www.docker.com/) (for end-to-end testing)

### Backend

```bash
cd backend
go build ./...          # compile
go test ./...           # all tests
go vet ./...            # vet
golangci-lint run       # lint
sqlc generate           # regenerate query code after schema changes
```

### Frontend

```bash
cd frontend
bun install             # install dependencies
bun run dev             # dev server on port 5173 (proxies /api to :8080)
bun run check           # lint + typecheck + unit tests
bun run e2e             # Playwright tests (starts its own backend and dev server)
```

### Plan generation eval

The offline suite runs plan generation against 8 scenarios and a 30-recipe library on in-process databases. It needs no network by default.

```bash
cd backend
go run ./cmd/plantry-eval                                  # heuristic chooser, 3 runs per scenario
go run ./cmd/plantry-eval -chooser llm                     # scripted LLM mode (fake client)
go run ./cmd/plantry-eval -compare earlier.jsonl           # diff against an earlier report
PLANTRY_EVAL_LIVE=1 PLANTRY_AI_PROVIDER=anthropic PLANTRY_AI_API_KEY=... \
  go run ./cmd/plantry-eval -chooser llm -model <model>    # real provider, costs money
```

It writes `plantry-eval.jsonl` (gitignored), prints a summary and exits non-zero when a run failed an expectation. Flags and scenarios: `backend/CLAUDE.md`.

### Merge gate

```bash
scripts/gate.sh             # go test -race, vet, golangci-lint, ARM cross builds, sqlc and deadcode checks,
                            # bun run check + build, then the full Playwright suite
```

`E2E_LANE=N` gives a worktree its own ports and database; `GATE_SKIP_BACKEND`, `GATE_SKIP_FRONTEND` and `GATE_SKIP_E2E` skip a part. The e2e suite never calls a real AI provider or the web: it runs the `fake` provider and a local fixture site for discovery (`frontend/e2e/fixtures/discovery-site`).

### Docker

```bash
docker compose up --build    # build and run
docker compose up            # run existing image
```

---

## Architecture

Plantry is a single Go binary with the React SPA embedded at build time.

```
plantry/
├── backend/            # Go — chi router, SQLite, goose migrations, sqlc
│   ├── cmd/plantry/    # Entry point
│   └── internal/
│       ├── adapters/   # SQLite repos, LLM clients, nutrition API adapters
│       ├── transport/  # HTTP handlers (thin translation layer)
│       └── config/     # Env var loading
└── frontend/           # React 19, TypeScript, Vite, TanStack Router/Query
    └── src/
        ├── routes/     # File-based routing (auto-generated tree)
        └── components/ # shadcn/ui primitives + domain components
```

**Backend:** hexagonal-lite — domain (pure Go, no DB/HTTP), adapters (infrastructure), transport (HTTP translation). Aggregates are plain exported structs; services hold all business logic.

**Frontend:** TanStack Router with file-based auto code-splitting; TanStack Query for server state; Tailwind v4 with CSS-variable design tokens.

**AI:** behind one `llm.Client` port (OpenAI, Anthropic, and a scripted fake for tests). Every call is metered into the run log. Plan generation, the assistant and discovery share it; see `docs/ARCHITECTURE.md` §7.

**Database:** Single SQLite file, WAL mode, managed by goose migrations. `sqlc` generates compile-time type-safe query code. After any schema or query change, run `sqlc generate`.

**Deployment:** Multi-stage Docker build — Bun builds the SPA → Go embeds the `dist/` output → Alpine runtime image (~30 MB). No CGO; the release image is built for `linux/amd64`, `linux/arm64` and `linux/arm/v7` (Raspberry Pi), cross-compiled natively. The version shown in Settings → Data & system comes from the `VERSION` build arg.

### Testing strategy

| Layer | Approach |
|---|---|
| Domain | Pure unit tests, no DB, no HTTP |
| Adapters | Real SQLite via `testhelper.NewTestDB()` — no mocks |
| Handlers | HTTP fixture tests |
| Frontend | Vitest unit tests |
| End-to-end | Playwright (requires backend + frontend running) |

> [!IMPORTANT]
> Adapter tests always hit a real SQLite database. Mocking the database is not done in this project.

---

## Design tokens

The UI uses the "Cookbook" palette: a paper-white ground, ink-blue accent, and six earthy batch colours. Headings use Newsreader, everything else Source Sans 3.

```css
--ground:  #FBFBFA;   /* page background */
--ink:     #1C2233;   /* primary text    */
--accent:  #27407A;   /* actions, focus  */
--line:    #E3E5EA;   /* borders         */
```

Full token list (light and dark): `docs/UX_DESIGN_SPEC.md` §3. Authoritative source: `frontend/src/index.css`.

---

## License

MIT — see [LICENSE](LICENSE).
