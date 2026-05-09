import {
  API,
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// Phase 2 of the plate workflow rework — read-only display of macros across
// the cell, slot sheet, and tray running total. The numbers come from the
// existing nutrition resolver; this spec asserts that the surfaces agree.
test.describe("Plate macros — Phase 2 read-only surfaces", () => {
  test("planned cell, slot sheet header, and tray running total all show kcal", async ({
    page,
    request,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 800)
    // Leaf with 200 kcal/100g; 200 g of it on a plate = 400 kcal.
    const rice = await seedLeafFood({
      name: `Rice ${tag}`,
      kcal_100g: 200,
    })
    // Composed reference: 1 portion uses 100 g rice → 200 kcal/portion.
    const dish = await seedComposedFood({
      name: `Dish ${tag}`,
      role: "main",
      reference_portions: 1,
      children: [
        {
          child_id: rice.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()

      // Open the picker tray for the empty cell. Target the tray sheet by
      // testid (not role="dialog") since other dialog-shaped overlays may
      // mount alongside it (popovers, etc).
      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet = page.getByTestId("tray-sheet")
      await expect(sheet).toBeVisible()
      await sheet.locator("input").first().fill(`Dish ${tag}`)

      // Stage the dish (1 portion) and assert the running total before commit.
      await sheet
        .getByRole("button", { name: new RegExp(`Dish ${tag}`) })
        .first()
        .click()
      const runningKcal = sheet.getByTestId("tray-running-kcal")
      await expect(runningKcal).toBeVisible()
      await expect(runningKcal).toContainText(/200/)

      // Commit the staged tray. We wait for the plate POST so the planner
      // cache invalidation has time to schedule the macros refetch.
      const platePost = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await sheet.getByTestId("tray-commit").click()
      await platePost
      await expect(sheet).not.toBeVisible()

      // 1) Cell shows kcal. Allow extra time because the macros endpoint
      // refetches after the plate POST settles, not synchronously with it.
      const kcalPill = cell.getByTestId("slot-cell-kcal")
      await expect(kcalPill).toBeVisible({ timeout: 10_000 })
      await expect(kcalPill).toContainText(/200/)

      // 2) Slot sheet header shows kcal — open it via the cell click target.
      await cell.getByTestId("slot-open-sheet").click()
      const slotSheet = page.getByTestId("slot-sheet")
      await expect(slotSheet).toBeVisible()
      const sheetKcal = slotSheet.getByTestId("slot-sheet-kcal")
      await expect(sheetKcal).toContainText(/200/)

      // 3) /api/plates/macros agrees with the displayed kcal.
      const today = new Date().toISOString().slice(0, 10)
      const macrosRes = await request.get(
        `${API}/api/plates/macros?from=${today}&to=${today}`
      )
      expect(macrosRes.ok()).toBeTruthy()
      const macrosBody = (await macrosRes.json()) as {
        plates: { plate_id: number; macros: { kcal: number } }[]
      }
      const seeded = macrosBody.plates[0]
      expect(seeded).toBeDefined()
      expect(Math.round(seeded.macros.kcal)).toBe(200)
    } finally {
      await cleanupFood(dish.id)
      await cleanupFood(rice.id)
      await cleanupSlot(slot.id)
    }
  })
})
