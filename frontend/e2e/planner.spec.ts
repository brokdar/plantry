import { expect, test } from "./helpers"

import {
  cleanupFood,
  cleanupSlot,
  pickAndCommitFood,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Weekly planner", () => {
  test("plan a meal, show it in the slot, navigate windows", async ({
    page,
  }) => {
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

      // Open the picker sheet from the empty cell.
      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet = page.getByRole("dialog")
      await expect(sheet).toBeVisible()

      // Search for the food, stage it, and commit the tray.
      await sheet.locator("input").first().fill(`Chicken curry ${tag}`)
      await pickAndCommitFood(page, sheet, new RegExp(`Chicken curry ${tag}`))

      // Hero title shows the component name.
      await expect(cell.getByText(`Chicken curry ${tag}`)).toBeVisible()

      // Navigate forward — cell still renders (just empty there).
      await page.getByRole("button", { name: /Next 7/i }).click()
      await expect(
        page.locator(`[data-testid="cell-0-${slot.id}"]`)
      ).toBeVisible()

      // Navigate back — plate is served from TanStack Query cache.
      await page.getByRole("button", { name: /Previous 7/i }).click()
      await expect(cell.getByText(`Chicken curry ${tag}`)).toBeVisible()
    } finally {
      await cleanupFood(main.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
