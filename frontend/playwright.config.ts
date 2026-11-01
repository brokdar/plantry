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
// Read by e2e/helpers.ts and e2e/global-setup.ts (workers inherit the env).
process.env.E2E_API_PORT = String(API_PORT)
process.env.E2E_DB_PATH = DB_PATH

// Specs that mutate process-global AI state (settings overrides, fake-script
// turn cursor). These must never run concurrently with each other, otherwise
// one spec's provider/model override corrupts another spec's chat stream.
const AI_SERIAL_SPECS = [
  "ai-chat.spec.ts",
  "ai-chat-advanced.spec.ts",
  "settings-ai.spec.ts",
  "keyboard-shortcuts.spec.ts",
  "settings-insights.spec.ts",
  "plan-wizard-llm.spec.ts",
]

// Specs that write the profile or plan settings through the UI. Those are
// global: the anchor moves every window other specs open, so they run last,
// alone, after the other projects.
const GLOBAL_SETTINGS_SPECS = [
  "plan-anchor-settings.spec.ts",
  "plan-generate-heuristic.spec.ts",
  "plan-review-llm.spec.ts",
  "plan-wizard.spec.ts",
  "profile.spec.ts",
  "settings-preferences.spec.ts",
  "today-freezer.spec.ts",
  "today-rate.spec.ts",
]

const SETUP_SPEC = "setup-wizard.spec.ts"

export default defineConfig({
  globalSetup: "./e2e/global-setup.ts",
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  // Locally capped: the default (half the cores) starts ~14 browsers.
  workers: process.env.CI ? 1 : 4,
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
      name: "chromium",
      testIgnore: [
        ...AI_SERIAL_SPECS,
        ...GLOBAL_SETTINGS_SPECS,
        SETUP_SPEC,
      ].map((s) => `**/${s}`),
      dependencies: ["setup"],
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "ai-serial",
      testMatch: AI_SERIAL_SPECS.map((s) => `**/${s}`),
      testIgnore: `**/${SETUP_SPEC}`,
      dependencies: ["setup"],
      fullyParallel: false,
      workers: 1,
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "global-settings",
      testMatch: GLOBAL_SETTINGS_SPECS.map((s) => `**/${s}`),
      dependencies: ["chromium", "ai-serial"],
      fullyParallel: false,
      workers: 1,
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: `cd ../backend && PLANTRY_PORT=${API_PORT} PLANTRY_DB_PATH=${DB_PATH} PLANTRY_IMAGE_PATH=${IMAGE_PATH} PLANTRY_LOG_LEVEL=error PLANTRY_DEV_MODE=1 PLANTRY_OFF_DISABLED=1 PLANTRY_SECRET_KEY=0123456789abcdef0123456789abcdef PLANTRY_AI_PROVIDER=fake PLANTRY_AI_MODEL=fake-e2e PLANTRY_AI_FAKE_SCRIPT=../frontend/e2e/fixtures/chat-scripts/plan-dinner.json go run ./cmd/plantry`,
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
  ],
})
