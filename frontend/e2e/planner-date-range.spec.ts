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

/**
 * Seed a plate via the new date-keyed POST /api/plates endpoint.
 * Returns the created plate id.
 */
async function seedPlateByDate(
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

/** Returns YYYY-MM-DD for today offset by `offsetDays`. */
function dateOffset(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

test.describe("Planner — date-range window", () => {
  test("renders a 7-day grid starting from anchor date (today)", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dr_grid_${tag}`, "Moon", 970)

    try {
      await page.goto("/")

      // 7 day-header columns must be present (idx 0–6)
      for (let i = 0; i < 7; i++) {
        await expect(page.getByTestId(`day-header-${i}`)).toBeVisible()
      }

      // Toolbar with navigator must be visible
      await expect(page.getByTestId("planner-toolbar")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("Next 7 button advances the window by 7 days", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dr_next_${tag}`, "Sun", 969)

    try {
      await page.goto("/")

      // Capture current range label before navigating
      const toolbar = page.getByTestId("planner-toolbar")
      await expect(toolbar).toBeVisible()

      const labelBefore = await page
        .locator(".min-w-48.text-center")
        .first()
        .textContent()

      const platesResp = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.getByRole("button", { name: /Next 7/i }).click()
      await platesResp

      // Range label must have changed
      const labelAfter = await page
        .locator(".min-w-48.text-center")
        .first()
        .textContent()
      expect(labelAfter).not.toBe(labelBefore)

      // Day columns are still 7
      for (let i = 0; i < 7; i++) {
        await expect(page.getByTestId(`day-header-${i}`)).toBeVisible()
      }
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("Prev 7 button returns to prior window", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dr_prev_${tag}`, "Star", 968)

    try {
      await page.goto("/")

      const labelBefore = await page
        .locator(".min-w-48.text-center")
        .first()
        .textContent()

      // Go forward one window
      const fwdResp = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.getByRole("button", { name: /Next 7/i }).click()
      await fwdResp

      // Now go back
      const backResp = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.getByRole("button", { name: /Previous 7/i }).click()
      await backResp

      const labelAfter = await page
        .locator(".min-w-48.text-center")
        .first()
        .textContent()
      expect(labelAfter).toBe(labelBefore)
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("Today button resets to offset 0", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dr_today_${tag}`, "Moon", 967)

    try {
      await page.goto("/")

      const labelAt0 = await page
        .locator(".min-w-48.text-center")
        .first()
        .textContent()

      // Advance two windows
      for (let i = 0; i < 2; i++) {
        const resp = page.waitForResponse(
          (r) =>
            r.url().includes("/api/plates") && r.request().method() === "GET"
        )
        await page.getByRole("button", { name: /Next 7/i }).click()
        await resp
      }

      const labelShifted = await page
        .locator(".min-w-48.text-center")
        .first()
        .textContent()
      expect(labelShifted).not.toBe(labelAt0)

      // Click "Today" to reset
      const resetResp = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.getByRole("button", { name: /^Today$/i }).click()
      await resetResp

      const labelReset = await page
        .locator(".min-w-48.text-center")
        .first()
        .textContent()
      expect(labelReset).toBe(labelAt0)
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("plate seeded for today's date appears in day-0 cell", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dr_seed_${tag}`, "Moon", 966)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Risotto ${tag}`, role: "main" },
      tag
    )

    const today = dateOffset(0)
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(today, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell.getByText(`Risotto ${tag}`)).toBeVisible()

      // Reload and verify persistence
      await page.reload()
      await expect(cell.getByText(`Risotto ${tag}`)).toBeVisible()
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("clearing a day removes all plates for that date", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dr_clear_${tag}`, "Sun", 965)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Gyoza ${tag}`, role: "main" },
      tag
    )

    const today = dateOffset(0)
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(today, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell.getByText(`Gyoza ${tag}`)).toBeVisible()

      // Hover day-0 header to reveal the clear button
      await page.getByTestId("day-header-0").hover()
      await page.getByTestId("clear-day-0").click()

      await expect(cell.getByText(`Gyoza ${tag}`)).toHaveCount(0)
      await expect(
        cell.getByRole("button", { name: /plan meal/i })
      ).toBeVisible()
      await expect(page.getByText("Day cleared")).toBeVisible()
      // Plate was cleared via UI — no further direct API cleanup needed
      plateId = undefined
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
