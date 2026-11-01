import {
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedWithStub,
  seedSlot,
  test,
  uid,
} from "./helpers"

test.describe("Add to plan (AC-10)", () => {
  test("picks the first free plan cell, creates a confirmed plate and offers Open plan", async ({
    page,
  }) => {
    const tag = uid()
    const { composed, stub } = await seedComposedWithStub(
      { name: `Add to plan ${tag}`, role: "main" },
      tag
    )
    // Parallel specs fill current-window cells and create and delete their
    // own slots, so the first free cell can be taken, or its slot gone, by
    // the time we submit. The plate goes into a slot only this test uses.
    const slot = await seedSlot(`slot.atp-${tag}`, "Sun", 993)
    try {
      await page.goto(`/recipes/${composed.id}`)
      await page.getByTestId("recipe-add-to-plan").click()
      const sheet = page.getByTestId("add-to-plan-sheet")
      await expect(sheet).toBeVisible()

      // The current and the next window are both on offer, and the first
      // radio (date and meal order) is preselected.
      await expect(sheet.getByRole("radio").first()).toBeChecked()

      const own = sheet.locator(`[data-testid$="|${slot.id}"]`).first()
      const key = await own.inputValue()
      const [date] = key.split("|")
      await sheet.locator("label", { has: own }).click()
      await expect(own).toBeChecked()

      const created = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/plates") && r.request().method() === "POST"
      )
      await sheet.getByTestId("add-to-plan-submit").click()
      const res = await created
      expect(res.status()).toBe(201)
      const plate = (await res.json()) as {
        id: number
        date: string
        slot_id: number
        state: string
        components: { food_id: number; cook_session_id?: number }[]
      }
      expect(plate).toMatchObject({
        date,
        slot_id: slot.id,
        state: "confirmed",
      })
      expect(plate.components).toHaveLength(1)
      expect(plate.components[0].food_id).toBe(composed.id)
      // A fresh session cooks it.
      expect(plate.components[0].cook_session_id).toBeGreaterThan(0)
      await expect(sheet).toHaveCount(0)

      const toast = page.locator("[data-sonner-toast]", {
        hasText: composed.name,
      })
      await toast.getByRole("button", { name: "Open plan" }).click()
      await expect(page).toHaveURL(new RegExp(`/plan\\?.*day=${date}`))
      await expect(page.locator(`[data-cell-key="${key}"]`)).toContainText(
        composed.name
      )
    } finally {
      // Also deletes the slot's plates.
      await cleanupSlot(slot.id)
      await cleanupFood(composed.id)
      await cleanupFood(stub.id)
    }
  })
})
