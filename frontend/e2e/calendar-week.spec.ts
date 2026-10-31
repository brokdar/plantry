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

/** Returns the ISO Monday (YYYY-MM-DD) of the week containing `date`. */
function mondayOf(date: Date): string {
  const d = new Date(date)
  const dow = d.getDay() // 0=Sun…6=Sat
  // Move back to Monday (weekStartsOn=1 default)
  d.setDate(d.getDate() - ((dow + 6) % 7))
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

test.describe("Calendar — week view", () => {
  test("week view shows the read-only grid without an edit mode", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.cal-wk-${tag}`, "Moon", 987)
    const { composed, stub } = await seedComposedWithStub(
      { name: `Week Dish ${tag}`, role: "main" },
      tag
    )

    // Use today's Monday so the plate is in the current week.
    const monday = mondayOf(new Date())
    // Pick the Monday itself as the plate date.
    const plateDate = monday

    let plateId = 0
    try {
      const plate = await seedPlate(plateDate, slot.id, composed.id)
      plateId = plate.id

      // Open week view for that Monday.
      await page.goto(`/calendar?mode=week&date=${monday}`)

      // The read-only grid; editing lives on the plan board (/plan).
      await expect(page).toHaveURL(/mode=week/)
      await expect(page.getByRole("button", { name: /^edit$/i })).toHaveCount(0)
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(composed.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
