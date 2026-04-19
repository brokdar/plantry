import { expect, test } from "./helpers"

import { cleanupSlot, seedSlot, uid } from "./helpers"

test.describe("Mobile planner (day-tab layout)", () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("renders day tabs and switches the visible day", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.breakfast_${tag}`, "Coffee", 994)

    try {
      await page.goto("/")

      // All 7 day tabs visible on mobile; desktop grid is hidden at this size.
      for (let i = 0; i < 7; i++) {
        await expect(page.getByTestId(`mobile-day-tab-${i}`)).toBeVisible()
      }

      // Switching the active day updates aria-selected.
      const wed = page.getByTestId("mobile-day-tab-2")
      await wed.click()
      await expect(wed).toHaveAttribute("aria-selected", "true")

      // Every active-day slot is reachable — seeded slot appears with its empty
      // placeholder button.
      await expect(
        page.locator(`[data-testid="cell-2-${slot.id}"]`).first()
      ).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
