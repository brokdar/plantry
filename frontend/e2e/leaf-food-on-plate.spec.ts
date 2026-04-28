// Payoff scenario for the unified Food refactor: a LEAF food (the old
// "Ingredient") can be placed directly on a plate without first wrapping it
// in a composed food. The shopping list aggregator and nutrition resolver
// both treat the leaf as 100 g per portion (the model convention).
//
// Two tests: one exercises the API path directly (fast, deterministic),
// the other exercises the picker UI "Lebensmittel" tab end-to-end.

import {
  API,
  apiRequest,
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
      const today = new Date().toISOString().slice(0, 10)

      // Create a plate on today's date, then attach the leaf food as a component.
      const plateRes = await ctx.post("/api/plates", {
        data: { date: today, slot_id: slot.id },
      })
      const plateBody = await plateRes.json()
      expect(
        plateRes.ok(),
        `Create plate failed: ${plateRes.status()} ${JSON.stringify(plateBody)}`
      ).toBeTruthy()
      plateId = (plateBody as { id: number }).id

      const compRes = await ctx.post(`/api/plates/${plateId}/components`, {
        data: { food_id: leaf.id, portions: 1 },
      })
      expect(
        compRes.ok(),
        `Add component failed: ${compRes.status()} ${JSON.stringify(await compRes.json())}`
      ).toBeTruthy()

      // Planner cell renders the leaf food's name.
      await page.goto("/")
      await page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.status() === 200
      )
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()
      await expect(cell.getByText(`Banana ${tag}`)).toBeVisible()

      // Shopping list contains exactly one row for the leaf food, weighted at
      // 100 g (one portion of a leaf = 100 g per the model convention).
      const tomorrow = new Date()
      tomorrow.setDate(tomorrow.getDate() + 1)
      const tomorrowStr = tomorrow.toISOString().slice(0, 10)
      const shopRes = await ctx.get(
        `/api/shopping-list?from=${today}&to=${tomorrowStr}`
      )
      expect(shopRes.ok()).toBeTruthy()
      const shop = (await shopRes.json()) as {
        items: { food_id: number; name: string; total_grams: number }[]
      }
      const row = shop.items.find((i) => i.food_id === leaf.id)
      expect(row, "shopping list missing leaf food row").toBeTruthy()
      expect(row?.name).toBe(`Banana ${tag}`)
      expect(row?.total_grams).toBeCloseTo(100, 1)

      // Nutrition range rolls up the leaf's per-100g macros (1 portion = 100g).
      const nutRes = await ctx.get(
        `/api/nutrition?from=${today}&to=${tomorrowStr}`
      )
      expect(nutRes.ok()).toBeTruthy()
      const nut = (await nutRes.json()) as {
        days: { date: string; macros: { kcal: number; protein: number } }[]
      }
      const dayEntry = nut.days.find((d) => d.date === today)
      expect(dayEntry, "no today nutrition entry").toBeTruthy()
      // Today's macros should include at least one banana's worth on top of any
      // pre-existing plates, so we assert the leaf's contribution is present
      // by lower-bounding kcal at 89 (a banana per 100 g).
      expect(dayEntry!.macros.kcal).toBeGreaterThanOrEqual(89)
      expect(dayEntry!.macros.protein).toBeGreaterThanOrEqual(1.1)
    } finally {
      if (plateId !== null) {
        await ctx.delete(`/api/plates/${plateId}`)
      }
      await ctx.dispose()
      await cleanupFood(leaf.id)
      await cleanupSlot(slot.id)
    }
  })

  test("picker sheet surfaces leaf food and saves it to the plate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.leaf_${tag}`, "Apple", 993)
    const leaf = await seedLeafFood({
      name: `Kiwi ${tag}`,
      kcal_100g: 61,
      protein_100g: 1.1,
    })

    try {
      await page.goto("/")
      await page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.status() === 200
      )

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()
      await cell.getByRole("button", { name: /plan meal/i }).click()

      // The picker opens as a right-side sheet — URL stays at /.
      await expect(page).toHaveURL(/\/$/)
      const sheet = page.getByRole("dialog")
      await expect(sheet).toBeVisible()

      // Search for the leaf food by name.
      await sheet.getByRole("textbox").fill(`Kiwi ${tag}`)

      // Food appears in the list; click it to create the plate.
      const createResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await sheet.getByRole("button", { name: `Kiwi ${tag}` }).click()
      await createResp

      // Sheet closes and the cell now shows the food.
      await expect(sheet).not.toBeVisible()
      await expect(cell.getByText(`Kiwi ${tag}`)).toBeVisible()
    } finally {
      // Best-effort plate cleanup via API before food/slot removal.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const today = new Date().toISOString().slice(0, 10)
      const platesRes = await ctx.get(`/api/plates?from=${today}&to=${today}`)
      if (platesRes.ok()) {
        const { plates } = (await platesRes.json()) as {
          plates: { id: number; slot_id: number }[]
        }
        for (const p of plates) {
          if (p.slot_id === slot.id) await ctx.delete(`/api/plates/${p.id}`)
        }
      }
      await ctx.dispose()
      await cleanupFood(leaf.id)
      await cleanupSlot(slot.id)
    }
  })
})
