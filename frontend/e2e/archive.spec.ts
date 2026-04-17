import { expect, request as apiRequest, test } from "@playwright/test"

import {
  API,
  cleanupComponent,
  cleanupIngredient,
  cleanupSlot,
  seedComponent,
  seedIngredient,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Archive + rotation insights", () => {
  test("past week appears in archive and opens a read-only grid", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.arc-${tag}`, "Moon", 997)
    const ing = await seedIngredient({ name: `Ing ${tag}`, kcal_100g: 100 })
    const comp = await seedComponent({
      name: `Archived Dish ${tag}`,
      role: "main",
      ingredients: [
        {
          ingredient_id: ing.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    let pastWeekId = 0
    const ctx = await apiRequest.newContext({ baseURL: API })
    try {
      // Create a past week via the by-date endpoint.
      const pastWeekRes = await ctx.get("/api/weeks/by-date?year=2025&week=1")
      expect(pastWeekRes.ok()).toBeTruthy()
      const pastWeek = (await pastWeekRes.json()) as { id: number }
      pastWeekId = pastWeek.id

      // Add a plate on the past week with our component.
      const plateRes = await ctx.post(`/api/weeks/${pastWeek.id}/plates`, {
        data: {
          day: 0,
          slot_id: slot.id,
          components: [{ component_id: comp.id, portions: 1 }],
        },
      })
      expect(plateRes.ok()).toBeTruthy()

      await page.goto("/archive")

      // The past week we just created is listed.
      const entry = page.getByTestId(`archive-week-${pastWeek.id}`)
      await expect(entry).toBeVisible()
      await expect(entry).toContainText("2025")

      // Navigate to the detail view.
      await entry.click()
      await expect(page).toHaveURL(new RegExp(`/archive/${pastWeek.id}$`))

      // Read-only grid renders with the plate's component name.
      await expect(page.getByText(`Archived Dish ${tag}`)).toBeVisible()

      // No edit affordances: no "Add a meal" buttons should exist.
      await expect(
        page.getByRole("button", { name: /Add a meal/i })
      ).toHaveCount(0)
    } finally {
      if (pastWeekId !== 0) {
        const det = await ctx.get(`/api/weeks/${pastWeekId}`)
        if (det.ok()) {
          const detail = (await det.json()) as { plates: { id: number }[] }
          for (const p of detail.plates) {
            await ctx.delete(`/api/plates/${p.id}`)
          }
        }
      }
      await ctx.dispose()
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
      await cleanupSlot(slot.id)
    }
  })

  test("component library surfaces Forgotten badge for never-cooked components", async ({
    page,
  }) => {
    const tag = uid()
    const ing = await seedIngredient({ name: `Ing ${tag}`, kcal_100g: 100 })
    const comp = await seedComponent({
      name: `Forgotten Dish ${tag}`,
      role: "main",
      ingredients: [
        {
          ingredient_id: ing.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await page.goto("/components")

      // Filter by searching for our unique component.
      await page.getByPlaceholder("Search components...").fill(`Dish ${tag}`)

      await expect(page.getByTestId(`badge-forgotten-${comp.id}`)).toBeVisible()
    } finally {
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
    }
  })
})
