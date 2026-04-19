import type { APIRequestContext } from "@playwright/test"

import { apiRequest, expect, test as baseTest, uid } from "./helpers"

const API = "http://localhost:8080"

const EDITABLE_KEYS = [
  "ai.provider",
  "ai.model",
  "ai.api_key",
  "ai.rate_limit_per_min",
  "ai.fake_script",
  "fdc.api_key",
] as const

/**
 * settingsApi is an APIRequestContext bound to the backend's /api prefix. It
 * is used both for test setup (seeding DB overrides) and teardown (clearing
 * every editable row). Exposed as a fixture so every test gets a fresh
 * handle and state is reset both before and after the test body runs — this
 * keeps the describe block safe even when a prior test throws before its
 * own afterEach can fire.
 */
type Fixtures = {
  settingsApi: APIRequestContext
  resetSettings: void
}

const test = baseTest.extend<Fixtures>({
  /* eslint-disable no-empty-pattern, react-hooks/rules-of-hooks */
  settingsApi: async ({}, use) => {
    const ctx = await apiRequest.newContext({ baseURL: API })
    await use(ctx)
    await ctx.dispose()
  },
  /* eslint-enable no-empty-pattern, react-hooks/rules-of-hooks */
  // Automatic fixture: runs for every test in this file, regardless of
  // whether the test references it. Resets app_settings to its env-only
  // state both before and after the test body so state from a failing
  // prior test can't leak into the next one.
  resetSettings: [
    async ({ settingsApi }, use) => {
      const reset = async () => {
        for (const key of EDITABLE_KEYS) {
          await settingsApi.delete(`/api/settings/${key}`)
        }
      }
      await reset()
      await use()
      await reset()
    },
    { auto: true },
  ],
})

/**
 * This spec mutates the process-global app_settings table. Tests in the same
 * describe block must run serially so their DELETE-then-PUT cycles don't
 * step on each other. Tests in other spec files do not touch /api/settings,
 * so cross-file parallelism remains safe.
 */
test.describe.configure({ mode: "serial" })

test.describe("AI settings", () => {
  test("shows env-sourced badges before any DB override exists", async ({
    page,
  }) => {
    await page.goto("/settings?tab=ai")

    await test.step("wizard rows render", async () => {
      await expect(
        page.getByRole("heading", { name: "Pick a provider" })
      ).toBeVisible()
      await expect(
        page.getByRole("heading", { name: "Supply an API key" })
      ).toBeVisible()
      await expect(
        page.getByRole("heading", { name: "Select a model" })
      ).toBeVisible()
    })

    await test.step("no DB overrides, ENV badges present", async () => {
      await expect(page.getByTestId("source-badge-db")).toHaveCount(0)
      await expect(page.getByTestId("source-badge-env").first()).toBeVisible()
    })
  })

  test("provider → key → model wizard persists DB overrides", async ({
    page,
  }) => {
    const tag = uid()

    await test.step("navigate to AI tab", async () => {
      await page.goto("/settings?tab=ai")
      await expect(
        page.getByRole("heading", { name: "Assistant Configuration" })
      ).toBeVisible()
    })

    await test.step("step 1 — select Anthropic provider", async () => {
      // Anthropic is a real value change from the env-sourced "fake", so the
      // Radix Select fires onValueChange reliably. Picking "fake" again
      // would be a no-op and leave us waiting for a PUT that never fires.
      const providerSelect = page.getByTestId("ai-provider-select")
      await providerSelect.click()

      const putProvider = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/settings/ai.provider") &&
          r.request().method() === "PUT" &&
          r.ok()
      )
      await page.getByRole("option", { name: "Anthropic", exact: true }).click()
      await putProvider

      await expect(page.getByTestId("ai-provider-select")).toContainText(
        "Anthropic"
      )
      await expect(page.getByTestId("clear-override-ai.provider")).toBeVisible()
    })

    await test.step("step 2 — validate and save API key", async () => {
      const keyInput = page.getByTestId("ai-api-key-input")
      await expect(keyInput).toBeEnabled()
      await keyInput.fill(`sk-ant-${tag}`)

      const getModels = page.waitForResponse(
        (r) =>
          r.url().includes("/api/settings/ai/models") &&
          r.request().method() === "GET" &&
          r.ok()
      )
      await page.getByTestId("ai-test-connection").click()
      await getModels

      await expect(
        page.getByText("Connection validated.", { exact: true })
      ).toBeVisible()

      const putKey = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/settings/ai.api_key") &&
          r.request().method() === "PUT" &&
          r.ok()
      )
      // After save completes the local draft is cleared and the query cache
      // is invalidated. Wait for the refetch to land before continuing so
      // the model dropdown's enablement reflects the persisted state.
      const refetchSettings = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/settings") &&
          r.request().method() === "GET" &&
          r.ok()
      )
      await page.getByTestId("ai-save-api-key").click()
      await putKey
      await refetchSettings
    })

    await test.step("step 3 — pick a Claude model", async () => {
      const modelSelect = page.getByTestId("ai-model-select")
      await expect(modelSelect).toBeEnabled()
      await modelSelect.click()

      const putModel = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/settings/ai.model") &&
          r.request().method() === "PUT" &&
          r.ok()
      )
      await page
        .getByRole("option", { name: "Claude Haiku 4.5", exact: true })
        .click()
      await putModel
    })

    await test.step("overrides persist across reload", async () => {
      await page.reload()
      await expect(page.getByTestId("ai-provider-select")).toContainText(
        "Anthropic"
      )
      // After reload the Test connection hasn't been re-run, so the dropdown
      // renders the stored id (raw) rather than the display name.
      await expect(page.getByTestId("ai-model-select")).toContainText(
        "claude-haiku-4-5"
      )
      // Provider, API key and model all show DB-sourced badges.
      await expect(page.getByTestId("source-badge-db")).toHaveCount(3)
      // The API-key row shows a masked preview built from the saved value —
      // strong evidence that the encrypted round-trip succeeded.
      await expect(
        page.getByText(new RegExp(`sk-\\*{4}${tag.slice(-4)}`))
      ).toBeVisible()
    })
  })

  test("clear override reverts a single row to its env source", async ({
    page,
    settingsApi,
  }) => {
    const seed = await settingsApi.put("/api/settings/ai.provider", {
      data: { value: "anthropic" },
    })
    expect(seed.ok()).toBeTruthy()

    await page.goto("/settings?tab=ai")
    // Exactly one DB badge — ai.provider.
    await expect(page.getByTestId("source-badge-db")).toHaveCount(1)
    await expect(page.getByTestId("ai-provider-select")).toContainText(
      "Anthropic"
    )

    const deleteProvider = page.waitForResponse(
      (r) =>
        r.url().endsWith("/api/settings/ai.provider") &&
        r.request().method() === "DELETE" &&
        r.ok()
    )
    await page.getByTestId("clear-override-ai.provider").click()
    await deleteProvider

    await expect(page.getByTestId("source-badge-db")).toHaveCount(0)
  })

  test("backend rejects invalid provider values with a translatable key", async ({
    settingsApi,
  }) => {
    const res = await settingsApi.put("/api/settings/ai.provider", {
      data: { value: "not_a_real_provider" },
    })
    expect(res.status()).toBe(400)
    const body = (await res.json()) as { message_key?: string }
    expect(body.message_key).toBe("error.settings.invalid_value")
  })

  test("system tab reports cipher availability", async ({ page }) => {
    await page.goto("/settings?tab=system")
    await expect(page.getByRole("heading", { name: "System" })).toBeVisible()
    // The playwright backend sets PLANTRY_SECRET_KEY so encryption is
    // enabled; the "Encryption disabled" banner must not appear.
    await expect(
      page.getByText("Encryption disabled", { exact: true })
    ).toHaveCount(0)
  })
})
