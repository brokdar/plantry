// Payoff scenario for the unified Food refactor: a LEAF food (the old
// "Ingredient") can be placed directly on a plate without first wrapping it
// in a composed food. The shopping list aggregator and nutrition resolver
// both treat the leaf as 100 g per portion (the model convention).
//
// This exercises the API path the planner UI relies on; the in-app picker
// currently filters to composed foods for UX reasons, so we POST the plate
// component over HTTP and assert the planner cell + shopping list +
// nutrition rollup all reflect it.

import {
  apiRequest,
  API,
  cleanupFood,
  cleanupSlot,
  expect,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

test.describe("Leaf food directly on a plate", () => {
  test("planner shows leaf, shopping list aggregates it, nutrition rolls up", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.snack_${tag}`, "Apple", 994)
    const leaf = await seedLeafFood({
      name: `Banana ${tag}`,
      kcal_100g: 89,
      protein_100g: 1.1,
      fat_100g: 0.3,
      carbs_100g: 22.8,
      fiber_100g: 2.6,
    })

    const ctx = await apiRequest.newContext({ baseURL: API })
    let plateId: number | null = null

    try {
      // Look up the current week so we can attach a plate to it.
      const weekRes = await ctx.get("/api/weeks/current")
      expect(weekRes.ok()).toBeTruthy()
      const week = (await weekRes.json()) as { id: number }

      // Create a plate at (day=0, slot=this) carrying the leaf food directly.
      const plateRes = await ctx.post(`/api/weeks/${week.id}/plates`, {
        data: {
          day: 0,
          slot_id: slot.id,
          components: [{ food_id: leaf.id, portions: 1 }],
        },
      })
      const plateBody = await plateRes.json()
      expect(
        plateRes.ok(),
        `Create plate failed: ${plateRes.status()} ${JSON.stringify(plateBody)}`
      ).toBeTruthy()
      plateId = (plateBody as { id: number }).id

      // Planner cell renders the leaf food's name.
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()
      await expect(cell.getByText(`Banana ${tag}`)).toBeVisible()

      // Shopping list contains exactly one row for the leaf food, weighted at
      // 100 g (one portion of a leaf = 100 g per the model convention).
      const shopRes = await ctx.get(`/api/weeks/${week.id}/shopping-list`)
      expect(shopRes.ok()).toBeTruthy()
      const shop = (await shopRes.json()) as {
        items: { food_id: number; name: string; total_grams: number }[]
      }
      const row = shop.items.find((i) => i.food_id === leaf.id)
      expect(row, "shopping list missing leaf food row").toBeTruthy()
      expect(row?.name).toBe(`Banana ${tag}`)
      expect(row?.total_grams).toBeCloseTo(100, 1)

      // Week nutrition rolls up the leaf's per-100g macros (1 portion = 100g).
      const nutRes = await ctx.get(`/api/weeks/${week.id}/nutrition`)
      expect(nutRes.ok()).toBeTruthy()
      const nut = (await nutRes.json()) as {
        days: { day: number; macros: { kcal: number; protein: number } }[]
      }
      const day0 = nut.days.find((d) => d.day === 0)
      expect(day0, "no day-0 nutrition entry").toBeTruthy()
      // Day-0 macros should include at least one banana's worth on top of any
      // pre-existing plates, so we assert the leaf's contribution is present
      // by lower-bounding kcal at 89 (a banana per 100 g).
      expect(day0!.macros.kcal).toBeGreaterThanOrEqual(89)
      expect(day0!.macros.protein).toBeGreaterThanOrEqual(1.1)
    } finally {
      if (plateId !== null) {
        await ctx.delete(`/api/plates/${plateId}`)
      }
      await ctx.dispose()
      await cleanupFood(leaf.id)
      await cleanupSlot(slot.id)
    }
  })
})
