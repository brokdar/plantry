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

**Weekly planning**
- 7-day × N-slot grid (desktop) or day scroll (mobile)
- Compose plates from your component library, swap sides in one click
- Saved templates for common compositions
- Copy a previous week's plan

**AI-assisted planning**
- Persistent chat panel per week with streaming responses
- The agent reads your library, profile, and nutrition targets — and actually edits your plan via tool calls
- Bring your own OpenAI or Anthropic key; AI degrades gracefully when no key is configured

**Nutrition tracking**
- Kcal + protein + fat + carbs per component, plate, day, and week
- Daily bars against targets; week summary
- Goal presets (cut / maintain / bulk) or custom macro split

**Component library**
- Create components with roles: `main`, `side_starch`, `side_veg`, `side_protein`, `sauce`, `drink`, `dessert`, `standalone`
- Variant groups link siblings (tofu curry ↔ chicken curry ↔ chickpea curry)
- Tags, images, prep/cook times, instructions

**Ingredient catalog**
- Barcode scan → auto-fetch from Open Food Facts
- Name search → USDA FoodData Central lookup
- Manual override always available

**Recipe import**
- Paste a URL → Plantry extracts name, portions, ingredients, instructions, and image
- Tries JSON-LD first (zero AI cost), falls back to LLM for unstructured pages

**Shopping list**
- Aggregates all ingredients across the week's plates
- Groups by ingredient, sums in base units (g / ml)

**Feedback loop**
- Mark each plate after cooking: cooked, skipped, loved, disliked
- AI reads your preferences before every generation

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
bun run e2e             # Playwright tests (requires a running backend)
```

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

**Database:** Single SQLite file, WAL mode, managed by goose migrations. `sqlc` generates compile-time type-safe query code. After any schema or query change, run `sqlc generate`.

**Deployment:** Multi-stage Docker build — Bun builds the SPA → Go embeds the `dist/` output → Alpine runtime image (~30 MB). No CGO; cross-compiles to ARMv7 and ARM64 for Raspberry Pi.

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
