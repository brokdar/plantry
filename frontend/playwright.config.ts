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
  "feedback.spec.ts",
]

export default defineConfig({
  globalSetup: "./e2e/global-setup.ts",
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: "list",
  use: {
    baseURL: `http://localhost:${PORT}`,
    locale: "en-US",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      testIgnore: AI_SERIAL_SPECS.map((s) => `**/${s}`),
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "ai-serial",
      testMatch: AI_SERIAL_SPECS.map((s) => `**/${s}`),
      fullyParallel: false,
      workers: 1,
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command:
        "cd ../backend && PLANTRY_DB_PATH=/tmp/plantry-e2e.db PLANTRY_IMAGE_PATH=/tmp/plantry-e2e-images PLANTRY_LOG_LEVEL=error PLANTRY_DEV_MODE=1 PLANTRY_SECRET_KEY=0123456789abcdef0123456789abcdef PLANTRY_AI_PROVIDER=fake PLANTRY_AI_MODEL=fake-e2e PLANTRY_AI_FAKE_SCRIPT=../frontend/e2e/fixtures/chat-scripts/plan-dinner.json go run ./cmd/plantry",
      url: "http://localhost:8080/api/health",
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: "bun run dev",
      url: `http://localhost:${PORT}`,
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
    },
  ],
})
