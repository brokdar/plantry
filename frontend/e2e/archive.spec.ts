import { expect, apiRequest, test } from "./helpers"

import {
  API,
  cleanupFood,
  cleanupSlot,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Archive", () => {
  test("past week's plate appears in agenda view after redirect from /archive", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.arc-${tag}`, "Moon", 997)
    const ing = await seedLeafFood({ name: `Ing ${tag}`, kcal_100g: 100 })
    const comp = await seedComposedFood({
      name: `Archived Dish ${tag}`,
      role: "main",
      children: [
        {
          child_id: ing.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    // Seed via the new date-keyed API with a date 30 days ago (within the
    // default 60-day agenda window).
    const date30DaysAgo = new Date()
    date30DaysAgo.setDate(date30DaysAgo.getDate() - 30)
    const plateDate = date30DaysAgo.toISOString().slice(0, 10)

    let plateId = 0
    const ctx = await apiRequest.newContext({ baseURL: API })
    try {
      // Create a plate via the new date-keyed endpoint.
      const plateRes = await ctx.post("/api/plates", {
        data: { date: plateDate, slot_id: slot.id },
      })
      expect(plateRes.ok()).toBeTruthy()
      const plate = (await plateRes.json()) as { id: number }
      plateId = plate.id

      // Add a component to the plate.
      const compRes = await ctx.post(`/api/plates/${plateId}/components`, {
        data: { food_id: comp.id, portions: 1 },
      })
      expect(compRes.ok()).toBeTruthy()

      // /archive redirects to /calendar?mode=agenda
      await page.goto("/archive")
      await expect(page).toHaveURL(/\/calendar.*mode=agenda/)

      // The agenda view renders (list container present).
      await expect(page.locator("details").first()).toBeVisible()
    } finally {
      if (plateId !== 0) {
        await ctx.delete(`/api/plates/${plateId}`)
      }
      await ctx.dispose()
      await cleanupFood(comp.id)
      await cleanupFood(ing.id)
      await cleanupSlot(slot.id)
    }
  })

  test("/archive redirects to /calendar?mode=agenda", async ({ page }) => {
    await page.goto("/archive")
    await expect(page).toHaveURL(/\/calendar.*mode=agenda/)
  })
})
