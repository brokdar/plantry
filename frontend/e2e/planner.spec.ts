import { expect, test } from "./helpers"

import {
  cleanupComponent,
  cleanupSlot,
  seedComponent,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Weekly planner", () => {
  test("plan a meal, swap a component, remove one, copy week", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 999)
    const main = await seedComponent({
      name: `Chicken curry ${tag}`,
      role: "main",
    })
    const side = await seedComponent({
      name: `Basmati ${tag}`,
      role: "side_starch",
    })
    const replacement = await seedComponent({
      name: `Naan ${tag}`,
      role: "side_starch",
    })

    try {
      await page.goto("/")

      // Empty cell at Mon (day=0) for the seeded slot. Click the "+" affordance.
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()

      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /add a meal/i }).click()
      // Sheet opens with role=main filter; pick the chicken curry.
      await page
        .getByRole("button", { name: new RegExp(`Chicken curry ${tag}`) })
        .click()
      await createPlateResp

      // Plate now shows the curry chip.
      await expect(cell.getByText(`Chicken curry ${tag}`)).toBeVisible()

      // Add a side via the plate's "Add component" button.
      const addCompResp = page.waitForResponse(
        (r) => /\/components$/.test(r.url()) && r.request().method() === "POST"
      )
      await cell
        .getByRole("button", { name: /add component/i })
        .first()
        .click()
      await page
        .getByRole("button", { name: new RegExp(`Basmati ${tag}`) })
        .click()
      await addCompResp

      await expect(cell.getByText(`Basmati ${tag}`)).toBeVisible()

      // Swap the basmati for naan via the swap button on the chip.
      const basmatiChip = cell.getByText(`Basmati ${tag}`).locator("..")
      const swapResp = page.waitForResponse(
        (r) =>
          /\/components\/\d+$/.test(r.url()) && r.request().method() === "PUT"
      )
      await basmatiChip.getByRole("button", { name: /swap/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Naan ${tag}`) })
        .click()
      await swapResp

      await expect(cell.getByText(`Naan ${tag}`)).toBeVisible()
      await expect(cell.getByText(`Basmati ${tag}`)).toHaveCount(0)

      // Remove the curry.
      const removeResp = page.waitForResponse(
        (r) =>
          /\/components\/\d+$/.test(r.url()) &&
          r.request().method() === "DELETE"
      )
      const curryChip = cell.getByText(`Chicken curry ${tag}`).locator("..")
      await curryChip.getByRole("button", { name: /remove/i }).click()
      await removeResp

      await expect(cell.getByText(`Chicken curry ${tag}`)).toHaveCount(0)

      // Navigate next week.
      await page.getByRole("button", { name: /next week/i }).click()
      // Empty cell on the next week (no plate yet — find by data-testid).
      await expect(
        page.locator(`[data-testid="cell-0-${slot.id}"]`)
      ).toBeVisible()

      // Navigate back.
      await page.getByRole("button", { name: /previous week/i }).click()
      await expect(cell.getByText(`Naan ${tag}`)).toBeVisible()
    } finally {
      await cleanupComponent(replacement.id)
      await cleanupComponent(side.id)
      await cleanupComponent(main.id)
      await cleanupSlot(slot.id)
    }
  })
})
