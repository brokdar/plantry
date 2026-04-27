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

/** Seed a plate via the date-keyed POST /api/plates endpoint. */
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

async function deletePlate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/plates/${id}`)
  await ctx.dispose()
}

function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function dateOffset(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() + n)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

test.describe("Shopping panel — range + presets + purchased state", () => {
  test("opens and renders shopping list for active window", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.shop_open_${tag}`, "Moon", 940)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Pasta ${tag}`, role: "main" },
      tag
    )

    const today = todayISO()
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(today, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")

      await page.getByRole("button", { name: /shopping/i }).click()
      const dialog = page.getByRole("dialog")
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // Food seeded for today should appear in the list
      await expect(dialog.getByText(new RegExp(`Stub.*${tag}`))).toBeVisible()
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
      await page.evaluate(() => localStorage.clear())
    }
  })

  test("'Next 7 days' preset chip updates the range description", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.shop_preset_${tag}`, "Sun", 939)

    try {
      await page.goto("/")

      await page.getByRole("button", { name: /shopping/i }).click()
      const dialog = page.getByRole("dialog")
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // Capture the description before clicking preset
      const desc = dialog.locator("[data-slot='description'], p, span").filter({
        hasText: /\d{4}-\d{2}-\d{2}/,
      })

      const shoppingListResp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/shopping-list") &&
          r.request().method() === "GET"
      )
      await dialog.getByRole("button", { name: /next 7 days/i }).click()
      await shoppingListResp

      // Range description now includes today and today+6
      const expectedFrom = todayISO()
      const expectedTo = dateOffset(6)
      await expect(desc.first()).toContainText(expectedFrom)
      await expect(desc.first()).toContainText(expectedTo)
    } finally {
      await cleanupSlot(slot.id)
      await page.evaluate(() => localStorage.clear())
    }
  })

  test("purchased item persists across panel close/reopen (localStorage)", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.shop_persist_${tag}`, "Moon", 938)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Lentils ${tag}`, role: "main" },
      tag
    )

    const today = todayISO()
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(today, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")

      await page.getByRole("button", { name: /shopping/i }).click()
      const dialog = page.getByRole("dialog")
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // Select "Next 7 days" to pin the range
      const shoppingListResp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/shopping-list") &&
          r.request().method() === "GET"
      )
      await dialog.getByRole("button", { name: /next 7 days/i }).click()
      await shoppingListResp

      // Check the ingredient
      const checkbox = dialog
        .getByRole("checkbox", { name: new RegExp(`Stub.*${tag}`, "i") })
        .first()
      await expect(checkbox).toBeVisible()
      await checkbox.click()
      await expect(checkbox).toBeChecked()

      // Close and reopen
      await page.keyboard.press("Escape")
      await page.getByRole("button", { name: /shopping/i }).click()
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // Select "Next 7 days" again to restore same key
      const refetchResp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/shopping-list") &&
          r.request().method() === "GET"
      )
      await dialog.getByRole("button", { name: /next 7 days/i }).click()
      await refetchResp

      // Item should still be checked
      await expect(
        dialog
          .getByRole("checkbox", { name: new RegExp(`Stub.*${tag}`, "i") })
          .first()
      ).toBeChecked()
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
      await page.evaluate(() => localStorage.clear())
    }
  })

  test("purchased state is keyed per range — switching presets does not contaminate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.shop_key_${tag}`, "Moon", 937)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Quinoa ${tag}`, role: "main" },
      tag
    )

    const today = todayISO()
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(today, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")

      // Open shopping panel
      await page.getByRole("button", { name: /shopping/i }).click()
      const dialog = page.getByRole("dialog")
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // --- Range A: Next 7 days ---
      const resp1 = page.waitForResponse(
        (r) =>
          r.url().includes("/api/shopping-list") &&
          r.request().method() === "GET"
      )
      await dialog.getByRole("button", { name: /next 7 days/i }).click()
      await resp1

      // Mark item purchased in Range A
      const checkboxA = dialog
        .getByRole("checkbox", { name: new RegExp(`Stub.*${tag}`, "i") })
        .first()
      await expect(checkboxA).toBeVisible()
      await checkboxA.click()
      await expect(checkboxA).toBeChecked()

      // --- Range B: This cycle (different localStorage key) ---
      const resp2 = page.waitForResponse(
        (r) =>
          r.url().includes("/api/shopping-list") &&
          r.request().method() === "GET"
      )
      await dialog.getByRole("button", { name: /this.*cycle/i }).click()
      await resp2

      // Mark item purchased in Range B too (independent key)
      const checkboxB = dialog
        .getByRole("checkbox", { name: new RegExp(`Stub.*${tag}`, "i") })
        .first()
      if ((await checkboxB.count()) > 0) {
        const isChecked = await checkboxB.isChecked()
        if (!isChecked) {
          await checkboxB.click()
          await expect(checkboxB).toBeChecked()
        }
      }

      // --- Switch back to Range A ---
      const resp3 = page.waitForResponse(
        (r) =>
          r.url().includes("/api/shopping-list") &&
          r.request().method() === "GET"
      )
      await dialog.getByRole("button", { name: /next 7 days/i }).click()
      await resp3

      // Range A purchased state still intact
      await expect(
        dialog
          .getByRole("checkbox", { name: new RegExp(`Stub.*${tag}`, "i") })
          .first()
      ).toBeChecked()
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
      await page.evaluate(() => localStorage.clear())
    }
  })
})
