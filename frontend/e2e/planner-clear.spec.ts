import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  mockAnchorToday,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

async function seedPlate(
  slotId: number,
  foodId: number,
  dayOffset: number = 0
): Promise<{ id: number }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const d = new Date()
  d.setDate(d.getDate() + dayOffset)
  const date = d.toISOString().slice(0, 10)
  const plateRes = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId },
  })
  const plate = (await plateRes.json()) as { id: number }
  await ctx.post(`/api/plates/${plate.id}/components`, {
    data: { food_id: foodId, portions: 1 },
  })
  await ctx.dispose()
  return plate
}

function localDate(dayOffset: number): string {
  const d = new Date()
  d.setDate(d.getDate() + dayOffset)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

async function setNoPlate(date: string, slotId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.put(`/api/availability/${date}/${slotId}`, {
    data: { mode: "none", remove_plate: true },
  })
  expect(res.ok(), `no plate ${date}/${slotId}: ${res.status()}`).toBeTruthy()
  await ctx.dispose()
}

async function clearOverride(date: string, slotId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/availability/${date}/${slotId}`)
  await ctx.dispose()
}

test.describe("Planner — clear shortcuts", () => {
  test.beforeEach(async ({ page }) => {
    await mockAnchorToday(page)
  })

  test("× quick-delete removes plate optimistically and shows undo toast", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.qdel_${tag}`, "Moon", 990)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const food = await seedComposedFood({
      name: `Salad ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await seedPlate(slot.id, food.id)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell.getByText(`Salad ${tag}`)).toBeVisible()

      await cell.hover()
      await cell.getByTestId("slot-quick-delete").click()

      // Optimistic: plate disappears immediately, empty state renders
      await expect(cell.getByText(`Salad ${tag}`)).toHaveCount(0)
      await expect(
        cell.getByRole("button", { name: /plan meal/i })
      ).toBeVisible()

      // Undo toast appears
      await expect(page.getByText("Plate deleted")).toBeVisible()
      await expect(
        page.getByRole("button", { name: "Undo", exact: true })
      ).toBeVisible()
    } finally {
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("undo quick-delete restores the plate", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.undo_${tag}`, "Moon", 989)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const food = await seedComposedFood({
      name: `Pasta ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await seedPlate(slot.id, food.id)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell.getByText(`Pasta ${tag}`)).toBeVisible()

      await cell.hover()
      await cell.getByTestId("slot-quick-delete").click()
      await expect(cell.getByText(`Pasta ${tag}`)).toHaveCount(0)

      // Click Undo before the 5 s window expires
      await page.getByRole("button", { name: "Undo", exact: true }).click()

      // Plate reappears (restored from snapshot — no server refetch)
      await expect(cell.getByText(`Pasta ${tag}`)).toBeVisible()
    } finally {
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("undoing one delete does not restore other pending deletes", async ({
    page,
  }) => {
    const tag = uid()
    // Two slots so two independent cells
    const slotA = await seedSlot(`slot.multi_a_${tag}`, "Moon", 982)
    const slotB = await seedSlot(`slot.multi_b_${tag}`, "Sun", 981)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const foodA = await seedComposedFood({
      name: `PlateA ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })
    const foodB = await seedComposedFood({
      name: `PlateB ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await seedPlate(slotA.id, foodA.id, 0)
      await seedPlate(slotB.id, foodB.id, 0)
      await page.goto("/")

      const cellA = page.locator(`[data-testid="cell-0-${slotA.id}"]`)
      const cellB = page.locator(`[data-testid="cell-0-${slotB.id}"]`)
      await expect(cellA.getByText(`PlateA ${tag}`)).toBeVisible()
      await expect(cellB.getByText(`PlateB ${tag}`)).toBeVisible()

      // Delete A
      await cellA.hover()
      await cellA.getByTestId("slot-quick-delete").click()
      await expect(cellA.getByText(`PlateA ${tag}`)).toHaveCount(0)

      // Delete B
      await cellB.hover()
      await cellB.getByTestId("slot-quick-delete").click()
      await expect(cellB.getByText(`PlateB ${tag}`)).toHaveCount(0)

      // Sonner prepends new toasts: [B_toast, A_toast]. B's toast is on top and
      // actionable; A's is stacked behind and covered. Click the accessible undo
      // (B's) and verify only B is restored while A stays removed.
      await page
        .getByRole("button", { name: "Undo", exact: true })
        .first()
        .click()
      await expect(cellB.getByText(`PlateB ${tag}`)).toBeVisible()
      await expect(cellA.getByText(`PlateA ${tag}`)).toHaveCount(0)
    } finally {
      await cleanupFood(foodA.id)
      await cleanupFood(foodB.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slotA.id)
      await cleanupSlot(slotB.id)
    }
  })

  test("clear row removes plates and leaves No plate cells alone", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.noplaterow_${tag}`, "Moon", 988)
    const day1 = localDate(1)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const food = await seedComposedFood({
      name: `Stew ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await seedPlate(slot.id, food.id, 0)
      await setNoPlate(day1, slot.id)
      await page.goto("/")

      const planned = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      const noPlate = page.locator(`[data-testid="cell-1-${slot.id}"]`)
      await expect(planned.getByText(`Stew ${tag}`)).toBeVisible()
      await expect(noPlate.locator('[data-slot-state="none"]')).toBeVisible()

      await page.getByTestId(`slot-row-menu-${slot.id}`).click()
      await page.getByTestId(`slot-row-clear-${slot.id}`).click()

      await expect(planned.getByText(`Stew ${tag}`)).toHaveCount(0)
      await expect(
        planned.getByRole("button", { name: /plan meal/i })
      ).toBeVisible()
      await expect(noPlate.locator('[data-slot-state="none"]')).toBeVisible()
    } finally {
      await clearOverride(day1, slot.id)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("clear day removes all plates for that day", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.clearday_${tag}`, "Sun", 987)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const food = await seedComposedFood({
      name: `Tacos ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await seedPlate(slot.id, food.id, 0)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell.getByText(`Tacos ${tag}`)).toBeVisible()

      // Open the day-0 header overflow menu and click "Clear day"
      await page.getByTestId("day-header-0").hover()
      await page.getByTestId("day-header-menu-0").click()
      await page.getByTestId("day-header-clear-0").click()

      await expect(cell.getByText(`Tacos ${tag}`)).toHaveCount(0)
      await expect(
        cell.getByRole("button", { name: /plan meal/i })
      ).toBeVisible()
      await expect(page.getByText("Day cleared")).toBeVisible()
      await expect(
        page.getByRole("button", { name: "Undo", exact: true })
      ).toBeVisible()
    } finally {
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("undo clear day restores plates", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.undoday_${tag}`, "Sun", 986)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const food = await seedComposedFood({
      name: `Ramen ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await seedPlate(slot.id, food.id, 0)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell.getByText(`Ramen ${tag}`)).toBeVisible()

      await page.getByTestId("day-header-0").hover()
      await page.getByTestId("day-header-menu-0").click()
      await page.getByTestId("day-header-clear-0").click()
      await expect(cell.getByText(`Ramen ${tag}`)).toHaveCount(0)

      await page.getByRole("button", { name: "Undo", exact: true }).click()

      await expect(cell.getByText(`Ramen ${tag}`)).toBeVisible()
    } finally {
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("clear day button absent when day has no plates", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.noplate_${tag}`, "Moon", 985)

    try {
      await page.goto("/")
      // Navigate to next week — guaranteed no plates, isolated from parallel
      // tests that seed plates in the current week's day-0.
      const nextWeekFetch = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.getByRole("button", { name: /Next 7/i }).click()
      await nextWeekFetch

      // No plates → the overflow menu button itself is absent (its only item
      // is "Clear day", which is gated on hasPlates).
      await page.getByTestId("day-header-0").hover()
      await expect(page.getByTestId("day-header-menu-0")).toHaveCount(0)
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("clear week removes all plates and shows undo toast", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.clearwk_${tag}`, "Star", 984)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const food = await seedComposedFood({
      name: `Curry ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await seedPlate(slot.id, food.id, 0)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell.getByText(`Curry ${tag}`)).toBeVisible()

      await page.getByTestId("planner-overflow").click()
      await page.getByTestId("clear-week").click()

      await expect(cell.getByText(`Curry ${tag}`)).toHaveCount(0)
      await expect(
        cell.getByRole("button", { name: /plan meal/i })
      ).toBeVisible()
      await expect(page.getByText("Week cleared")).toBeVisible()
      await expect(
        page.getByRole("button", { name: "Undo", exact: true })
      ).toBeVisible()
    } finally {
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("undo clear week restores plates", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.undowk_${tag}`, "Star", 983)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const food = await seedComposedFood({
      name: `Sushi ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await seedPlate(slot.id, food.id, 0)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell.getByText(`Sushi ${tag}`)).toBeVisible()

      await page.getByTestId("planner-overflow").click()
      await page.getByTestId("clear-week").click()
      await expect(cell.getByText(`Sushi ${tag}`)).toHaveCount(0)

      await page.getByRole("button", { name: "Undo", exact: true }).click()

      await expect(cell.getByText(`Sushi ${tag}`)).toBeVisible()
    } finally {
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
