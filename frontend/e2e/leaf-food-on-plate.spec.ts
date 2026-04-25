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
      await page.waitForResponse(
        (r) => r.url().includes("/api/weeks") && r.status() === 200
      )
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

  test("picker Lebensmittel tab surfaces leaf food and saves it to the plate", async ({
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
        (r) => r.url().includes("/api/weeks") && r.status() === 200
      )

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()
      await cell.getByRole("button", { name: /plan meal/i }).click()
      await expect(page).toHaveURL(
        new RegExp(`/planner/\\d+/0/${slot.id}/pick`)
      )

      // Switch to the Lebensmittel (leaf) tab.
      await page.getByTestId("picker-tab-leaf").click()

      // Seeded leaf food appears as a picker card.
      await expect(page.getByTestId(`picker-card-${leaf.id}`)).toBeVisible()

      // Click the card — it lands in the tray.
      await page.getByTestId(`picker-card-${leaf.id}`).click()
      await expect(page.getByTestId(`tray-item-${leaf.id}`)).toBeVisible()

      // Save creates the plate and returns to the planner.
      const createResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await page.getByTestId("tray-save").click()
      await createResp

      await expect(page).toHaveURL(/\/$/)
      await expect(cell.getByText(`Kiwi ${tag}`)).toBeVisible()
    } finally {
      // Best-effort plate cleanup via API before food/slot removal.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const weekRes = await ctx.get("/api/weeks/current")
      if (weekRes.ok()) {
        const { id: weekId } = (await weekRes.json()) as { id: number }
        const det = await ctx.get(`/api/weeks/${weekId}`)
        if (det.ok()) {
          const detail = (await det.json()) as {
            plates: { id: number; slot_id: number }[]
          }
          for (const p of detail.plates) {
            if (p.slot_id === slot.id) await ctx.delete(`/api/plates/${p.id}`)
          }
        }
      }
      await ctx.dispose()
      await cleanupFood(leaf.id)
      await cleanupSlot(slot.id)
    }
  })
})
