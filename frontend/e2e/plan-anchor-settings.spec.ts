import { expect, setSetting, test } from "./helpers"

test.describe("Plan anchor settings", () => {
  // The setup project sets plan.anchor = today for every spec; restore it.
  test.afterEach(async () => {
    await setSetting("plan.anchor", "today")
  })

  test("Settings → Cooking offers exactly the two anchors", async ({
    page,
  }) => {
    await page.goto("/settings/cooking")

    const radios = page.locator('input[name="plan-anchor"]')
    await expect(radios).toHaveCount(2)
    await expect(page.getByTestId("plan-anchor-radio-today")).toBeVisible()
    await expect(
      page.getByTestId("plan-anchor-radio-next_shopping_day")
    ).toBeVisible()
    await expect(page.getByTestId("plan-week-starts-on-select")).toHaveCount(0)
  })

  test("picking an anchor saves it", async ({ page }) => {
    // Start from "today" so the first pick is always a change.
    await setSetting("plan.anchor", "today")
    await page.goto("/settings/cooking")

    for (const anchor of ["next_shopping_day", "today"] as const) {
      const saved = page.waitForResponse(
        (r) =>
          r.url().includes("/api/settings/plan.anchor") &&
          r.request().method() === "PUT"
      )
      await page.getByTestId(`plan-anchor-radio-${anchor}`).click()
      expect((await saved).ok()).toBe(true)
      await expect(
        page.getByTestId(`plan-anchor-radio-${anchor}`)
      ).toBeChecked()
    }
  })
})
