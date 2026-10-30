// Phase 3 — kind-aware quantity controls in the planner picker.
//
// This spec exercises the picker tray to confirm composed foods get an
// integer stepper (no fractional portions).

import {
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedWithStub,
  seedSlot,
  test,
  uid,
} from "./helpers"

test.describe("Plate quantity — kind-aware controls", () => {
  test("composed food picker shows an integer stepper", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.q_composed_${tag}`, "Apple", 991)
    const seeded = await seedComposedWithStub(
      { name: `CurryQ ${tag}`, role: "main", reference_portions: 2 },
      tag
    )

    try {
      await page.goto("/")
      await page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.status() === 200
      )

      const cell = page.getByTestId(`cell-0-${slot.id}`).first()
      await expect(cell).toBeVisible()
      await cell.getByRole("button", { name: /plan meal/i }).click()

      const sheet = page.getByTestId("tray-sheet")
      await expect(sheet).toBeVisible()

      await sheet.getByTestId("tray-search").fill(`CurryQ ${tag}`)
      await sheet
        .getByRole("button", { name: new RegExp(`CurryQ ${tag}`) })
        .first()
        .click()

      // Composed → integer stepper. The staged row exposes a spinbutton with
      // aria-valuenow=1 and the displayed text must contain "×1".
      const sb = sheet.getByRole("spinbutton")
      await expect(sb).toHaveAttribute("aria-valuenow", "1")
      await expect(sb).toContainText("×1")

      // − is disabled at 1 (no fractional portions).
      const minus = sheet.getByRole("button", { name: /−1/ })
      await expect(minus).toBeDisabled()

      // + bumps to 2 then 3.
      await sheet.getByRole("button", { name: /\+1/ }).click()
      await expect(sb).toHaveAttribute("aria-valuenow", "2")
      await sheet.getByRole("button", { name: /\+1/ }).click()
      await expect(sb).toHaveAttribute("aria-valuenow", "3")
    } finally {
      await cleanupFood(seeded.composed.id)
      await cleanupFood(seeded.stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
