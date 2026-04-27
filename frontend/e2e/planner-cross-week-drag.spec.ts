/**
 * Planner — cross-window drag spec
 *
 * DnD with dnd-kit is notoriously hard to simulate reliably in Playwright
 * because dnd-kit uses PointerEvents with a 6px activation constraint and
 * does not respond to the Playwright mouse helpers the same way a native
 * HTML5 drag does.
 *
 * Strategy: test the underlying PUT /api/plates/{id} move operation directly
 * (the same mutation that onDragEnd fires). Then verify the planner grid
 * reflects the updated date after a page reload. This tests the full
 * data contract (API → query cache → grid render) without depending on
 * fragile pointer simulation.
 *
 * A single skipped test documents the aspirational DnD simulation approach
 * so it can be enabled once a helper / dnd-kit test util exists.
 */

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

/** Seed a plate via POST /api/plates (date-keyed, phase 2 API). */
async function seedPlateByDate(
  date: string,
  slotId: number,
  foodId: number
): Promise<{ id: number }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId },
  })
  const plate = (await res.json()) as { id: number }
  await ctx.post(`/api/plates/${plate.id}/components`, {
    data: { food_id: foodId, portions: 1 },
  })
  await ctx.dispose()
  return plate
}

/** Move a plate to a new date via PUT /api/plates/{id}. */
async function movePlateToDate(
  plateId: number,
  newDate: string
): Promise<void> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.put(`/api/plates/${plateId}`, { data: { date: newDate } })
  await ctx.dispose()
}

async function deletePlate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/plates/${id}`)
  await ctx.dispose()
}

/** Returns YYYY-MM-DD for today offset by `offsetDays`. */
function dateOffset(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

test.describe("Planner — cross-window plate move (API + grid verification)", () => {
  test("moving a plate to the next day via API reflects in the grid", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.move_${tag}`, "Moon", 950)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Pho ${tag}`, role: "main" },
      tag
    )

    const day0 = dateOffset(0)
    const day1 = dateOffset(1)
    let plateId: number | undefined

    try {
      // Seed on day 0
      const plate = await seedPlateByDate(day0, slot.id, food.id)
      plateId = plate.id

      // Verify it appears in day-0 cell
      await page.goto("/")
      const cell0 = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell0.getByText(`Pho ${tag}`)).toBeVisible()

      // Move via API (simulating what DnD onDragEnd fires)
      await movePlateToDate(plateId, day1)

      // Reload — grid must show plate in day-1, absent from day-0
      await page.reload()

      const cell1 = page.locator(`[data-testid="cell-1-${slot.id}"]`).first()
      await expect(cell1.getByText(`Pho ${tag}`)).toBeVisible()
      await expect(cell0.getByText(`Pho ${tag}`)).toHaveCount(0)
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("moving a plate to the previous day via API reflects in the grid", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.moveback_${tag}`, "Sun", 949)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Ramen ${tag}`, role: "main" },
      tag
    )

    // Seed on day 2 so there is a valid day-1 to move back to within the window
    const day1 = dateOffset(1)
    const day2 = dateOffset(2)
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(day2, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")
      const cell2 = page.locator(`[data-testid="cell-2-${slot.id}"]`).first()
      await expect(cell2.getByText(`Ramen ${tag}`)).toBeVisible()

      // Move back one day
      await movePlateToDate(plateId, day1)

      await page.reload()

      const cell1 = page.locator(`[data-testid="cell-1-${slot.id}"]`).first()
      await expect(cell1.getByText(`Ramen ${tag}`)).toBeVisible()
      await expect(cell2.getByText(`Ramen ${tag}`)).toHaveCount(0)
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("PUT /api/plates/{id} with new date returns 200 and updated date field", async () => {
    const tag = uid()
    const slot = await seedSlot(`slot.putdate_${tag}`, "Star", 948)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Dumpling ${tag}`, role: "main" },
      tag
    )

    const day0 = dateOffset(0)
    const day3 = dateOffset(3)
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(day0, slot.id, food.id)
      plateId = plate.id

      const ctx = await apiRequest.newContext({ baseURL: API })
      const res = await ctx.put(`/api/plates/${plateId}`, {
        data: { date: day3 },
      })
      await ctx.dispose()

      expect(res.status()).toBe(200)
      const body = (await res.json()) as { date: string }
      expect(body.date).toBe(day3)
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  // Aspirational: direct PointerEvent DnD simulation.
  // dnd-kit uses a 6px activation constraint on PointerSensor which makes
  // Playwright's mouse.move() unreliable — the drag often never activates.
  // Enable this once a dnd-kit Playwright helper or accessibility-tree
  // (keyboard) drag is wired up.
  test.skip("drag plate from day-0 to day-1 via pointer events", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dnd_${tag}`, "Moon", 947)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Steak ${tag}`, role: "main" },
      tag
    )

    const day0 = dateOffset(0)
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(day0, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")

      const src = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      const dst = page.locator(`[data-testid="cell-1-${slot.id}"]`).first()

      await expect(src.getByText(`Steak ${tag}`)).toBeVisible()

      const srcBox = await src.boundingBox()
      const dstBox = await dst.boundingBox()
      if (!srcBox || !dstBox) throw new Error("bounding box missing")

      await page.mouse.move(
        srcBox.x + srcBox.width / 2,
        srcBox.y + srcBox.height / 2
      )
      await page.mouse.down()
      // Move past the 6px activation constraint
      await page.mouse.move(
        srcBox.x + srcBox.width / 2 + 10,
        srcBox.y + srcBox.height / 2
      )
      await page.mouse.move(
        dstBox.x + dstBox.width / 2,
        dstBox.y + dstBox.height / 2
      )
      await page.mouse.up()

      const dst1 = page.locator(`[data-testid="cell-1-${slot.id}"]`).first()
      await expect(dst1.getByText(`Steak ${tag}`)).toBeVisible()
      await expect(src.getByText(`Steak ${tag}`)).toHaveCount(0)
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
