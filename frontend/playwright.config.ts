import { defineConfig, devices } from "@playwright/test"

const PORT = 5173

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
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command:
        "cd ../backend && PLANTRY_DB_PATH=/tmp/plantry-e2e.db PLANTRY_IMAGE_PATH=/tmp/plantry-e2e-images PLANTRY_LOG_LEVEL=error PLANTRY_DEV_MODE=1 PLANTRY_AI_PROVIDER=fake PLANTRY_AI_MODEL=fake-e2e PLANTRY_AI_FAKE_SCRIPT=../frontend/e2e/fixtures/chat-scripts/plan-dinner.json go run ./cmd/plantry",
      url: "http://localhost:8080/api/health",
      reuseExistingServer: !process.env.CI,
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
