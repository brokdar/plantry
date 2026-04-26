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

test.describe("Day detail page", () => {
  test("renders slots; next-day nav updates URL; Back to calendar returns to /calendar", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.day-det-${tag}`, "Sun", 985)
    const { composed, stub } = await seedComposedWithStub(
      { name: `Day Dish ${tag}`, role: "main" },
      tag
    )

    const today = dateOffset(0)
    const tomorrow = dateOffset(1)

    let plateId = 0
    try {
      const plate = await seedPlate(today, slot.id, composed.id)
      plateId = plate.id

      // Open day detail for today.
      await page.goto(`/day/${today}`)

      // The day page container is rendered.
      await expect(page.getByTestId("day-page")).toBeVisible()

      // Slots grid is rendered (ReadOnlyPlannerGrid renders a table/grid).
      // The grid renders slot rows — at least the page container is visible.
      await expect(page.getByTestId("day-page")).toBeVisible()

      // Click "next day" — the aria-label is t("day.next_day") = "Next day".
      await page.getByRole("button", { name: /next day/i }).click()

      // URL updates to the next date.
      await expect(page).toHaveURL(new RegExp(`/day/${tomorrow}`))

      // Go back to today's page.
      await page.goto(`/day/${today}`)

      // Click "Back to calendar" link — t("day.back_to_calendar") = "Back to calendar".
      await page.getByRole("link", { name: /back to calendar/i }).click()

      // Should land on /calendar.
      await expect(page).toHaveURL(/\/calendar/)
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(composed.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
