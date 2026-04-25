import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

async function seedPlate(
  slotId: number,
  foodId: number,
  day: number = 0
): Promise<{ id: number }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const weekRes = await ctx.get("/api/weeks/current")
  const week = (await weekRes.json()) as { id: number }
  const plateRes = await ctx.post(`/api/weeks/${week.id}/plates`, {
    data: {
      day,
      slot_id: slotId,
      components: [{ food_id: foodId, portions: 1 }],
    },
  })
  const plate = (await plateRes.json()) as { id: number }
  await ctx.dispose()
  return plate
}

async function seedSkippedPlate(
  slotId: number,
  foodId: number,
  day: number = 0
) {
  const plate = await seedPlate(slotId, foodId, day)
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.post(`/api/plates/${plate.id}/skip`, {
    data: { skipped: true, note: null },
  })
  await ctx.dispose()
  return plate
}

test.describe("Planner — clear shortcuts", () => {
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
      await expect(page.getByRole("button", { name: "Undo" })).toBeVisible()
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
      await page.getByRole("button", { name: "Undo" }).click()

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

      // Undo only A — B must stay removed. Sonner prepends new toasts, so PlateA's (first) is last in DOM.
      await page.getByRole("button", { name: "Undo" }).last().click()
      await expect(cellA.getByText(`PlateA ${tag}`)).toBeVisible()
      await expect(cellB.getByText(`PlateB ${tag}`)).toHaveCount(0)
    } finally {
      await cleanupFood(foodA.id)
      await cleanupFood(foodB.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slotA.id)
      await cleanupSlot(slotB.id)
    }
  })

  test("× button on skipped slot removes it and shows undo toast", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.skipdel_${tag}`, "Moon", 988)
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
      await seedSkippedPlate(slot.id, food.id)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell.locator('[data-slot-state="skipped"]')).toBeVisible()

      await cell.hover()
      await cell.getByTestId("slot-quick-delete").click()

      // Cell returns to empty state
      await expect(
        cell.getByRole("button", { name: /plan meal/i })
      ).toBeVisible()
      await expect(page.getByText("Plate deleted")).toBeVisible()
    } finally {
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

      // Hover day-0 header to reveal the clear button
      await page.getByTestId("day-header-0").hover()
      await page.getByTestId("clear-day-0").click()

      await expect(cell.getByText(`Tacos ${tag}`)).toHaveCount(0)
      await expect(
        cell.getByRole("button", { name: /plan meal/i })
      ).toBeVisible()
      await expect(page.getByText("Day cleared")).toBeVisible()
      await expect(page.getByRole("button", { name: "Undo" })).toBeVisible()
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
      await page.getByTestId("clear-day-0").click()
      await expect(cell.getByText(`Ramen ${tag}`)).toHaveCount(0)

      await page.getByRole("button", { name: "Undo" }).click()

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

      // Hover day-0 header — button not rendered since no plates exist
      await page.getByTestId("day-header-0").hover()
      await expect(page.getByTestId("clear-day-0")).toHaveCount(0)
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

      await page.getByTestId("clear-week").click()

      await expect(cell.getByText(`Curry ${tag}`)).toHaveCount(0)
      await expect(
        cell.getByRole("button", { name: /plan meal/i })
      ).toBeVisible()
      await expect(page.getByText("Week cleared")).toBeVisible()
      await expect(page.getByRole("button", { name: "Undo" })).toBeVisible()
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

      await page.getByTestId("clear-week").click()
      await expect(cell.getByText(`Sushi ${tag}`)).toHaveCount(0)

      await page.getByRole("button", { name: "Undo" }).click()

      await expect(cell.getByText(`Sushi ${tag}`)).toBeVisible()
    } finally {
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
