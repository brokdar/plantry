import { API, apiRequest, expect, test } from "./helpers"

/**
 * Settings and the profile are process-global, so this spec runs in the
 * serial `ai-serial` project (with profile.spec.ts) and restores everything
 * it changes in `finally`.
 */

const SETTING_KEYS = ["plan.default_keep_days", "plan.freezing_enabled"]

type StoredProfile = Record<string, unknown> & {
  diet_type: string
  household_portions: number
}

async function api() {
  return apiRequest.newContext({ baseURL: API })
}

async function getProfile(): Promise<StoredProfile> {
  const ctx = await api()
  const body = (await (await ctx.get("/api/profile")).json()) as StoredProfile
  await ctx.dispose()
  return body
}

async function getSettings(): Promise<Record<string, string | undefined>> {
  const ctx = await api()
  const body = (await (await ctx.get("/api/settings")).json()) as {
    items: { key: string; value?: string }[]
  }
  await ctx.dispose()
  return Object.fromEntries(body.items.map((i) => [i.key, i.value]))
}

async function restore(profile: StoredProfile) {
  const ctx = await api()
  const input = { ...profile }
  delete input.updated_at
  await ctx.put("/api/profile", { data: input })
  for (const key of SETTING_KEYS) {
    await ctx.delete(`/api/settings/${key}`)
  }
  await ctx.dispose()
}

test.describe("Settings preferences persist", () => {
  test("diet type, household portions and two cooking settings survive a reload", async ({
    page,
  }) => {
    const original = await getProfile()
    try {
      const isPut =
        (path: string) =>
        (r: { url(): string; request(): { method(): string } }) =>
          r.url().endsWith(path) && r.request().method() === "PUT"

      await test.step("You & household: vegetarian, one more child", async () => {
        await page.goto("/settings/household")
        const diet = page.waitForResponse(isPut("/api/profile"))
        await page.getByText("Vegetarian", { exact: true }).click()
        await diet
        await expect(
          page.getByText("Plantry plans no meat or fish.")
        ).toBeVisible()

        const portions = page.waitForResponse(isPut("/api/profile"))
        await page.getByRole("button", { name: "Increase Children" }).click()
        await portions
      })

      await test.step("Cooking & variety: keep days +1, freezing on", async () => {
        await page.getByTestId("settings-nav-cooking").click()
        await expect(page).toHaveURL(/\/settings\/cooking$/)

        const keep = page.waitForResponse(
          isPut("/api/settings/plan.default_keep_days")
        )
        await page
          .getByRole("button", { name: "Increase Leftovers keep for" })
          .click()
        await keep

        const freezing = page.waitForResponse(
          isPut("/api/settings/plan.freezing_enabled")
        )
        // click(), not check(): the switch is controlled by the query cache,
        // which re-renders a tick after the click.
        await page.getByLabel("Freezing").click()
        await freezing
        await expect(page.getByLabel("Freezing")).toBeChecked()
        await expect(page.getByTestId("freezer-keep-input")).toBeVisible()
      })

      await test.step("reload shows the stored values", async () => {
        await page.reload()
        await expect(
          page.getByRole("group", { name: "Leftovers keep for" })
        ).toContainText("4 days")
        await expect(page.getByLabel("Freezing")).toBeChecked()

        await page.goto("/settings/household")
        await expect(
          page.getByRole("radio", { name: "Vegetarian" })
        ).toBeChecked()
        await expect(
          page.getByText(
            `Each plate is planned for ${original.household_portions + 0.5} portions.`,
            { exact: false }
          )
        ).toBeVisible()
      })

      await test.step("the API agrees", async () => {
        const profile = await getProfile()
        expect(profile.diet_type).toBe("vegetarian")
        expect(profile.household_portions).toBe(
          original.household_portions + 0.5
        )
        const settings = await getSettings()
        expect(settings["plan.default_keep_days"]).toBe("4")
        expect(settings["plan.freezing_enabled"]).toBe("true")
      })
    } finally {
      await restore(original)
    }
  })
})
