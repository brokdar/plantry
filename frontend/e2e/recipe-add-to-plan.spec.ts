import {
  API,
  apiRequest,
  cleanupFood,
  expect,
  seedComposedWithStub,
  test,
  uid,
} from "./helpers"

async function deletePlate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/plates/${id}`)
  await ctx.dispose()
}

test.describe("Add to plan (AC-10)", () => {
  test("picks the first free plan cell, creates a confirmed plate and offers Open plan", async ({
    page,
  }) => {
    const tag = uid()
    const { composed, stub } = await seedComposedWithStub(
      { name: `Add to plan ${tag}`, role: "main" },
      tag
    )
    let plateId: number | null = null
    try {
      await page.goto(`/recipes/${composed.id}`)
      await page.getByTestId("recipe-add-to-plan").click()
      const sheet = page.getByTestId("add-to-plan-sheet")
      await expect(sheet).toBeVisible()

      // The current and the next window are both on offer, and the first
      // radio (date and meal order) is preselected.
      const radios = sheet.getByRole("radio")
      await expect(radios.first()).toBeChecked()
      const key = await sheet.getByRole("radio", { checked: true }).inputValue()
      const [date, slot] = key.split("|")

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
      plateId = plate.id
      expect(plate).toMatchObject({
        date,
        slot_id: Number(slot),
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
      await expect(
        page.locator(`[data-cell-key="${date}|${slot}"]`)
      ).toContainText(composed.name)
    } finally {
      if (plateId !== null) await deletePlate(plateId)
      await cleanupFood(composed.id)
      await cleanupFood(stub.id)
    }
  })
})
