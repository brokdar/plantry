import {
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedWithStub,
  seedSlot,
  test,
  uid,
} from "./helpers"

test.describe("Planner picker sheet", () => {
  test("empty cell opens picker sheet and clicking food creates plate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 996)
    const { composed: main, stub: mainStub } = await seedComposedWithStub(
      { name: `Sushi ${tag}`, role: "main" },
      tag
    )

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()

      // Empty cell opens the picker sheet (not a route navigation).
      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet = page.getByRole("dialog")
      await expect(sheet).toBeVisible()

      // URL stays at /.
      await expect(page).toHaveURL(/\/$/)

      // Search for the food.
      await sheet.locator("input").first().fill(`Sushi ${tag}`)

      // Clicking the food button creates the plate immediately — no tray-save.
      const createResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await sheet
        .getByRole("button", { name: new RegExp(`Sushi ${tag}`) })
        .click()
      await createResp

      // Sheet closes and the cell shows the food name.
      await expect(sheet).not.toBeVisible()
      await expect(cell.getByText(`Sushi ${tag}`)).toBeVisible()
    } finally {
      await cleanupFood(main.id)
      await cleanupFood(mainStub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("search input filters the food list", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 995)
    const { composed: food1, stub: stub1 } = await seedComposedWithStub(
      { name: `Apricot ${tag}`, role: "main" },
      tag
    )
    const { composed: food2, stub: stub2 } = await seedComposedWithStub(
      { name: `Blueberry ${tag}`, role: "main" },
      `${tag}b`
    )

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet = page.getByRole("dialog")
      await expect(sheet).toBeVisible()

      // Search for food1 — food2 must not appear.
      await sheet.locator("input").first().fill(`Apricot ${tag}`)
      await expect(
        sheet.getByRole("button", { name: new RegExp(`Apricot ${tag}`) })
      ).toBeVisible()
      await expect(
        sheet.getByRole("button", { name: new RegExp(`Blueberry ${tag}`) })
      ).toHaveCount(0)

      // Close without picking.
      await page.keyboard.press("Escape")
    } finally {
      await cleanupFood(food1.id)
      await cleanupFood(stub1.id)
      await cleanupFood(food2.id)
      await cleanupFood(stub2.id)
      await cleanupSlot(slot.id)
    }
  })
})
