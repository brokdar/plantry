import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedWithStub,
  seedSlot,
  test,
  uid,
} from "./helpers"

/** Returns YYYY-MM-DD for today offset by `offsetDays`. */
function dateOffset(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

async function seedPlate(
  date: string,
  slotId: number,
  foodId: number
): Promise<{ id: number }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId, food_id: foodId },
  })
  const body = await res.json()
  await ctx.dispose()
  return body as { id: number }
}

async function deletePlate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/plates/${id}`)
  await ctx.dispose()
}

test.describe("Calendar — month view", () => {
  test("seeded plate shows in month cell; cell click navigates to day detail; mode toggle switches to week", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.cal-mo-${tag}`, "Sun", 988)
    const { composed, stub } = await seedComposedWithStub(
      { name: `Month Dish ${tag}`, role: "main" },
      tag
    )

    // Seed a plate 30 days ago so it falls in a past month.
    const plateDate = dateOffset(-30)
    const monthParam = plateDate.slice(0, 7) // YYYY-MM

    let plateId = 0
    try {
      const plate = await seedPlate(plateDate, slot.id, composed.id)
      plateId = plate.id

      // Open the month view for that month.
      await page.goto(`/calendar?mode=month&date=${monthParam}`)

      // The cell for that date should be rendered (button with data-date).
      const cell = page.locator(`button[data-date="${plateDate}"]`)
      await expect(cell).toBeVisible()

      // The cell contains at least one plate preview (1× or note text).
      await expect(cell.locator("div").first()).toBeVisible()

      // Click the cell — URL should change to /day/<date>.
      await cell.click()
      await expect(page).toHaveURL(new RegExp(`/day/${plateDate}`))

      // Day page renders.
      await expect(page.getByTestId("day-page")).toBeVisible()

      // Go back to calendar.
      await page.goBack()
      await expect(page).toHaveURL(/\/calendar/)

      // Toggle mode to Week — the mode toggle button labelled "Week" should be present.
      await page.getByRole("button", { name: "Week" }).click()
      await expect(page).toHaveURL(/mode=week/)
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(composed.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
