import { API, apiRequest, expect, test } from "./helpers"

// KIT-7 (A2): the kitchen is visible and editable without a provider key, and
// what is saved survives a reload.
test.describe("Kitchen settings", () => {
  test("Settings → Kitchen saves skill, equipment and a leaning", async ({
    page,
  }) => {
    const ctx = await apiRequest.newContext({ baseURL: API })
    const before = (await (await ctx.get("/api/kitchen")).json()) as Record<
      string,
      unknown
    >
    try {
      await page.goto("/settings/kitchen")
      await expect(page.getByTestId("settings-section-kitchen")).toBeVisible()
      await expect(page.getByTestId("kitchen-minutes-weekday")).toBeVisible()

      await page.getByTestId("option-kitchen-skill-confident").click()
      await expect(page.getByLabel("Wok")).not.toBeChecked()
      await page.getByLabel("Wok").click()
      await expect(page.getByLabel("Wok")).toBeChecked()
      await page.getByLabel("Flavour leanings").fill("smoky")
      await page.getByLabel("Flavour leanings").press("Enter")
      await expect(page.getByText("smoky", { exact: true })).toBeVisible()

      await page.reload()
      await expect(page.getByRole("radio", { name: "Confident" })).toBeChecked()
      await expect(page.getByLabel("Wok")).toBeChecked()
      await expect(page.getByText("smoky", { exact: true })).toBeVisible()
    } finally {
      await ctx.put("/api/kitchen", {
        data: {
          minutes_weekday: before.minutes_weekday,
          minutes_weekend: before.minutes_weekend,
          skill: before.skill,
          equipment: before.equipment,
          heat: before.heat,
          leanings: before.leanings,
        },
      })
      await ctx.dispose()
    }
  })
})
