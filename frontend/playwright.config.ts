import { defineConfig, devices } from "@playwright/test"

// E2E_LANE (0–9) gives each parallel worktree its own backend port, Vite port,
// DB and image dir, so several agents can run e2e at once. Lane 0 keeps the
// historical defaults (8080 / 5173 / /tmp/plantry-e2e.db).
const LANE = Number(process.env.E2E_LANE ?? 0)
const API_PORT = 8080 + LANE
const PORT = 5173 + LANE
const DB_PATH = LANE ? `/tmp/plantry-e2e-${LANE}.db` : "/tmp/plantry-e2e.db"
const IMAGE_PATH = LANE
  ? `/tmp/plantry-e2e-images-${LANE}`
  : "/tmp/plantry-e2e-images"
// The global-settings specs change settings every other spec reads, so they
// get their own backend, Vite and DB and run beside the rest instead of after
// it (they were a serial 56 s at the end of the run).
const API_PORT_GLOBAL = 8280 + LANE
const PORT_GLOBAL = 5273 + LANE
const DB_PATH_GLOBAL = `/tmp/plantry-e2e-global-${LANE}.db`
const IMAGE_PATH_GLOBAL = `/tmp/plantry-e2e-images-global-${LANE}`
// Read by e2e/helpers.ts (workers inherit the env).
process.env.E2E_API_PORT = String(API_PORT)
// The discovery fixture site (Phase 5 D20): serves the recipe pages the fake
// research client points at. Read by e2e/discovery-fixture.ts.
const SITE_PORT = 9100 + LANE
process.env.E2E_SITE_PORT = String(SITE_PORT)

// Specs that mutate process-global AI state (settings overrides, fake-script
// turn cursor). These must never run concurrently with each other, otherwise
// one spec's provider/model override corrupts another spec's chat stream.
const AI_SERIAL_SPECS = [
  "assistant-panel.spec.ts",
  "assistant-move.spec.ts",
  "recipe-chat.spec.ts",
  "discovery.spec.ts",
  "settings-ai.spec.ts",
  "keyboard-shortcuts.spec.ts",
  "taste-page.spec.ts",
  "settings-insights.spec.ts",
]

// Specs that write the profile or plan settings through the UI. Those are
// global: the anchor moves every window other specs open, so they run last,
// alone, after the other projects. `meal-away.spec.ts` is not here: Away all
// day writes a cell for every slot, other specs' slots included, but it works
// in its own `futureWindow()` band, and a revert skips a cell whose slot a
// parallel spec has deleted since (availability.Service.Revert), so it runs
// with the parallel chromium specs.
const GLOBAL_SETTINGS_SPECS = [
  "assistant-drafts.spec.ts",
  "chef-find.spec.ts",
  "chef-tips.spec.ts",
  "import-check.spec.ts",
  "discovery-explore-plan.spec.ts",
  "plan-anchor-settings.spec.ts",
  "plan-generate-heuristic.spec.ts",
  "plan-replan.spec.ts",
  "plan-week-ledger.spec.ts",
  "plan-review-llm.spec.ts",
  "plan-wizard.spec.ts",
  "profile.spec.ts",
  "settings-chef.spec.ts",
  "settings-insights-generate.spec.ts",
  "settings-preferences.spec.ts",
  "today-freezer.spec.ts",
  "today-rate.spec.ts",
]

const SETUP_SPEC = "setup-wizard.spec.ts"
// Z5 §17.1: needs the install still fresh (no recipes), so it runs right
// after `setup` and before everything else, and removes what it makes.
const FRESH_SPEC = "fresh-install.spec.ts"

export default defineConfig<object, { apiPort: number }>({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  // Locally capped: the default (half the cores) starts ~14 browsers.
  // CI: the runner has 4 vCPUs and 16 GB (serial CI e2e took 6 min). Locally 6:
  // chromium takes 4, the serial ai-serial and global-settings run beside it.
  workers: process.env.CI ? 4 : 6,
  reporter: "list",
  use: {
    baseURL: `http://localhost:${PORT}`,
    locale: "en-US",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [
    // Plan D22: walks the setup wizard on the fresh DB (so `/` stops
    // redirecting to /setup) and sets `plan.anchor = today` (D21). Every
    // other project depends on it; don't run with `--no-deps`.
    {
      name: "setup",
      testMatch: `**/${SETUP_SPEC}`,
      fullyParallel: false,
      workers: 1,
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "fresh-install",
      testMatch: `**/${FRESH_SPEC}`,
      dependencies: ["setup"],
      fullyParallel: false,
      workers: 1,
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "chromium",
      testIgnore: [
        ...AI_SERIAL_SPECS,
        ...GLOBAL_SETTINGS_SPECS,
        SETUP_SPEC,
        FRESH_SPEC,
      ].map((s) => `**/${s}`),
      dependencies: ["fresh-install"],
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "ai-serial",
      testMatch: AI_SERIAL_SPECS.map((s) => `**/${s}`),
      testIgnore: `**/${SETUP_SPEC}`,
      dependencies: ["fresh-install"],
      fullyParallel: false,
      workers: 1,
      use: { ...devices["Desktop Chrome"] },
    },
    // The global-settings server walks the setup wizard itself, then its
    // specs run one at a time, beside chromium and ai-serial. Playwright starts
    // a project only after every project of the dependency level before it, so
    // global-settings also waits for fresh-install: on the level of chromium
    // and ai-serial, not before them.
    {
      name: "setup-global",
      testMatch: `**/${SETUP_SPEC}`,
      fullyParallel: false,
      use: {
        ...devices["Desktop Chrome"],
        baseURL: `http://localhost:${PORT_GLOBAL}`,
        apiPort: API_PORT_GLOBAL,
      },
    },
    {
      name: "global-settings",
      testMatch: GLOBAL_SETTINGS_SPECS.map((s) => `**/${s}`),
      dependencies: ["setup-global", "fresh-install"],
      fullyParallel: false,
      workers: 1,
      use: {
        ...devices["Desktop Chrome"],
        baseURL: `http://localhost:${PORT_GLOBAL}`,
        apiPort: API_PORT_GLOBAL,
      },
    },
  ],
  webServer: [
    // The AI rate limit (10 a minute by default) is lifted: the ai-serial
    // specs chat back to back, and at --repeat-each the default turned
    // chats away mid-run.
    {
      // The database is wiped here, before the server opens it: a wipe after the
      // server started (a globalSetup) unlinks the open file, and the next
      // pooled connection reopens an empty database ("no such table" 500s).
      // The server log goes to /tmp/plantry-e2e-server-<lane>.log (level error).
      command: `rm -f ${DB_PATH} ${DB_PATH}-wal ${DB_PATH}-shm && cd ../backend && PLANTRY_PORT=${API_PORT} PLANTRY_DB_PATH=${DB_PATH} PLANTRY_IMAGE_PATH=${IMAGE_PATH} PLANTRY_LOG_LEVEL=error PLANTRY_DEV_MODE=1 PLANTRY_OFF_DISABLED=1 PLANTRY_SECRET_KEY=0123456789abcdef0123456789abcdef PLANTRY_AI_PROVIDER=fake PLANTRY_AI_MODEL=fake-e2e PLANTRY_AI_RATE_LIMIT_PER_MIN=1000 PLANTRY_AI_FAKE_SCRIPT=../frontend/e2e/fixtures/chat-scripts/plan-dinner.json go run ./cmd/plantry >>/tmp/plantry-e2e-server-${LANE}.log 2>&1`,
      url: `http://localhost:${API_PORT}/api/health`,
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: `PLANTRY_API_TARGET=http://localhost:${API_PORT} bun run dev --port ${PORT} --strictPort`,
      url: `http://localhost:${PORT}`,
      // Never reuse: a Vite on this port may belong to another worktree.
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: `rm -f ${DB_PATH_GLOBAL} ${DB_PATH_GLOBAL}-wal ${DB_PATH_GLOBAL}-shm && cd ../backend && PLANTRY_PORT=${API_PORT_GLOBAL} PLANTRY_DB_PATH=${DB_PATH_GLOBAL} PLANTRY_IMAGE_PATH=${IMAGE_PATH_GLOBAL} PLANTRY_LOG_LEVEL=error PLANTRY_DEV_MODE=1 PLANTRY_OFF_DISABLED=1 PLANTRY_SECRET_KEY=0123456789abcdef0123456789abcdef PLANTRY_AI_PROVIDER=fake PLANTRY_AI_MODEL=fake-e2e PLANTRY_AI_RATE_LIMIT_PER_MIN=1000 PLANTRY_AI_FAKE_SCRIPT=../frontend/e2e/fixtures/chat-scripts/plan-dinner.json go run ./cmd/plantry >>/tmp/plantry-e2e-server-global-${LANE}.log 2>&1`,
      url: `http://localhost:${API_PORT_GLOBAL}/api/health`,
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: `PLANTRY_API_TARGET=http://localhost:${API_PORT_GLOBAL} bun run dev --port ${PORT_GLOBAL} --strictPort`,
      url: `http://localhost:${PORT_GLOBAL}`,
      // Never reuse: a Vite on this port may belong to another worktree.
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: `E2E_SITE_PORT=${SITE_PORT} bun e2e/fixtures/discovery-site/serve.ts`,
      url: `http://localhost:${SITE_PORT}/pasta.html`,
      reuseExistingServer: false,
      timeout: 30_000,
    },
  ],
})
