import { defineConfig, devices } from "@playwright/test"

const PORT = 5173

// Specs that mutate process-global AI state (settings overrides, fake-script
// turn cursor). These must never run concurrently with each other, otherwise
// one spec's provider/model override corrupts another spec's chat stream.
const AI_SERIAL_SPECS = [
  "ai-chat.spec.ts",
  "ai-chat-advanced.spec.ts",
  "settings-ai.spec.ts",
  "keyboard-shortcuts.spec.ts",
  "generate-plan.spec.ts",
]

// Specs that write the profile or plan settings through the UI. Those are
// global: the anchor moves every window other specs open, so they run last,
// alone, after the other projects.
const GLOBAL_SETTINGS_SPECS = [
  "plan-anchor-settings.spec.ts",
  "profile.spec.ts",
  "settings-preferences.spec.ts",
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
      command:
        "cd ../backend && PLANTRY_DB_PATH=/tmp/plantry-e2e.db PLANTRY_IMAGE_PATH=/tmp/plantry-e2e-images PLANTRY_LOG_LEVEL=error PLANTRY_DEV_MODE=1 PLANTRY_OFF_DISABLED=1 PLANTRY_SECRET_KEY=0123456789abcdef0123456789abcdef PLANTRY_AI_PROVIDER=fake PLANTRY_AI_MODEL=fake-e2e PLANTRY_AI_FAKE_SCRIPT=../frontend/e2e/fixtures/chat-scripts/plan-dinner.json go run ./cmd/plantry",
      url: "http://localhost:8080/api/health",
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: "bun run dev",
      url: `http://localhost:${PORT}`,
      // Vite has no state; safe to reuse a running dev server. The backend
      // (above) is always spawned fresh so e2e tests never silently bind to
      // a developer's real DB or a stale process.
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
    },
  ],
})
