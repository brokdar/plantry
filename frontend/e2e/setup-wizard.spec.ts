import { API, apiRequest, expect, setSetting, test } from "./helpers"

/**
 * Plan D22: the `setup` project. Runs once, serially, against the fresh e2e
 * DB (no slots, so setup is not completed), walks the wizard with its
 * defaults, and leaves the install set up for every other project.
 */

async function getJSON<T>(path: string): Promise<T> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(path)
  expect(res.ok()).toBe(true)
  const body = (await res.json()) as T
  await ctx.dispose()
  return body
}

test.describe("Setup wizard", () => {
  test("a fresh install walks Next to Finish and lands on Recipes", async ({
    page,
  }) => {
    await test.step("/ sends a new install to /setup", async () => {
      await page.goto("/")
      await expect(page).toHaveURL(/\/setup\/?$/)
      await expect(page.getByText("Step 1 of 7")).toBeVisible()
    })

    await test.step("Next through every step with the defaults", async () => {
      const next = page.getByTestId("setup-next")
      for (let step = 2; step <= 7; step++) {
        await next.click()
        await expect(page.getByText(`Step ${step} of 7`)).toBeVisible()
        if (step === 4) {
          // On a desktop column the grid fits without scrolling.
          const box = page.getByRole("grid").locator("..")
          await expect
            .poll(() => box.evaluate((el) => el.scrollWidth - el.clientWidth))
            .toBeLessThanOrEqual(0)
          // The availability grid scrolls inside its own box on a phone.
          await page.setViewportSize({ width: 375, height: 800 })
          await expect
            .poll(() =>
              page.evaluate(() => document.documentElement.scrollWidth)
            )
            .toBeLessThanOrEqual(375)
          await page.setViewportSize({ width: 1280, height: 720 })
        }
      }
      await expect(
        page.getByText("You can add this later. Planning works without it.")
      ).toBeVisible()
      await next.click()
      await expect(
        page.getByRole("heading", { name: "You're set up" })
      ).toBeVisible()
    })

    await test.step("Add recipes completes setup and opens Recipes", async () => {
      const completed = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/setup/complete") &&
          r.request().method() === "POST"
      )
      await page.getByRole("button", { name: "Add recipes" }).click()
      expect((await completed).ok()).toBe(true)
      await expect(page).toHaveURL(/\/components\/?$/)
      // The guided state, with the real count of a fresh DB.
      const guide = page.getByTestId("recipe-onboarding")
      await expect(guide).toBeVisible()
      await expect(guide.getByText(/^0 of 10 recipes/)).toBeVisible()
      // Focus lands on the page, not on <body>. (Checked once Recipes has
      // rendered: until then the shell briefly wraps the wizard's own main.)
      await expect(page.getByRole("main")).toBeFocused()
    })

    await test.step("the defaults are stored", async () => {
      const slots = await getJSON<{
        items: { id: number; name_key: string; active: boolean }[]
      }>("/api/settings/slots")
      expect(slots.items.map((s) => s.name_key)).toEqual([
        "slot.breakfast",
        "slot.lunch",
        "slot.dinner",
      ])

      const profile = await getJSON<Record<string, unknown>>("/api/profile")
      expect(profile).toMatchObject({
        kcal_target: 2200,
        protein_pct: 30,
        fat_pct: 30,
        carbs_pct: 40,
      })

      const defaults = await getJSON<{ items: { mode: string }[] }>(
        "/api/availability/defaults"
      )
      expect(defaults.items).toHaveLength(21)
      expect(defaults.items.every((c) => c.mode === "plan")).toBe(true)

      // The count is real: a fresh DB has no recipes.
      const status = await getJSON<{
        completed: boolean
        recipe_count: number
      }>("/api/setup")
      expect(status).toMatchObject({ completed: true, recipe_count: 0 })
    })

    await test.step("/ opens Today now", async () => {
      await page.goto("/")
      await expect(page).toHaveURL(/\/today\/?$/)
      // Below the recipe target Today says how many recipes are missing.
      await expect(page.getByTestId("today-setup-card")).toContainText(
        "Add 10 more recipes to start planning"
      )
    })

    // Plan D21: the planner specs seed plates on today's dates.
    await setSetting("plan.anchor", "today")
  })
})
