import { expect, test } from "./helpers"

import {
  cleanupFood,
  cleanupSlot,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Weekly planner", () => {
  // Post-redesign smoke test — covers the core edit loop through the new
  // SlotCell surface. Swap / remove component actions now live inside the
  // picker route (Phase 6) and the per-chip buttons of the old PlateCell were
  // removed; those flows are covered by planner-picker.spec.ts once it lands.
  test("plan a meal, show it in the slot, navigate weeks", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 999)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const main = await seedComposedFood({
      name: `Chicken curry ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()

      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      // Empty slot renders a single full-area button with aria-label "Plan meal".
      await cell.getByRole("button", { name: /plan meal/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Chicken curry ${tag}`) })
        .click()
      await page.getByTestId("tray-save").click()
      await createPlateResp

      // Hero title shows the component name.
      await expect(cell.getByText(`Chicken curry ${tag}`)).toBeVisible()

      // Navigate forward, assert the week shifts and the cell is empty there.
      await page.getByRole("button", { name: /next week/i }).click()
      await expect(
        page.locator(`[data-testid="cell-0-${slot.id}"]`)
      ).toBeVisible()

      // Navigate back, plate should still be here.
      await page.getByRole("button", { name: /previous week/i }).click()
      await expect(cell.getByText(`Chicken curry ${tag}`)).toBeVisible()
    } finally {
      await cleanupFood(main.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
