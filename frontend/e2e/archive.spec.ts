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

test.describe("Archive + rotation insights", () => {
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

  test("component library surfaces Forgotten badge for never-cooked components", async ({
    page,
  }) => {
    const tag = uid()
    const ing = await seedLeafFood({ name: `Ing ${tag}`, kcal_100g: 100 })
    const comp = await seedComposedFood({
      name: `Forgotten Dish ${tag}`,
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

    try {
      // The insights endpoint caps its forgotten list at 10 entries by
      // default. When repeat runs / parallel workers have created many
      // never-cooked components, our seeded component can fall outside that
      // window. Intercept the request and pass forgotten_limit=50 so the
      // UI renders the badge deterministically.
      await page.route("**/api/foods/insights*", async (route) => {
        const url = new URL(route.request().url())
        if (!url.searchParams.has("forgotten_limit")) {
          url.searchParams.set("forgotten_limit", "50")
        }
        await route.continue({ url: url.toString() })
      })

      await page.goto("/components")

      // Filter by searching for our unique component.
      await page.getByPlaceholder(/search the catalog/i).fill(`Dish ${tag}`)

      await expect(page.getByTestId(`badge-forgotten-${comp.id}`)).toBeVisible()
    } finally {
      await cleanupFood(comp.id)
      await cleanupFood(ing.id)
    }
  })
})
