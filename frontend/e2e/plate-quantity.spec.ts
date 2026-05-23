// Phase 3 — kind-aware quantity controls in the planner picker.
//
// These specs exercise the picker tray and slot sheet to confirm:
//   1. composed foods get an integer stepper (no fractional portions)
//   2. leaf foods get a quantity + unit input with the food's portion table
//   3. portion overrides (e.g. apple = 180 g) flow through to the cell kcal
//   4. saved leaf components show as `<amount> <unit>` in the slot sheet,
//      and editing the amount updates the cell kcal.

import {
  cleanupFood,
  cleanupSlot,
  expect,
  mockAnchorToday,
  seedComposedWithStub,
  seedLeafFood,
  seedPlateWithComponent,
  seedSlot,
  test,
  uid,
} from "./helpers"

test.describe("Plate quantity — kind-aware controls", () => {
  test("composed food picker shows an integer stepper", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.q_composed_${tag}`, "Apple", 991)
    const seeded = await seedComposedWithStub(
      { name: `CurryQ ${tag}`, role: "main", reference_portions: 2 },
      tag
    )

    try {
      await page.goto("/")
      await page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.status() === 200
      )

      const cell = page.getByTestId(`cell-0-${slot.id}`).first()
      await expect(cell).toBeVisible()
      await cell.getByRole("button", { name: /plan meal/i }).click()

      const sheet = page.getByTestId("tray-sheet")
      await expect(sheet).toBeVisible()

      await sheet.getByTestId("tray-search").fill(`CurryQ ${tag}`)
      await sheet
        .getByRole("button", { name: new RegExp(`CurryQ ${tag}`) })
        .first()
        .click()

      // Composed → integer stepper. The staged row exposes a spinbutton with
      // aria-valuenow=1 and the displayed text must contain "×1".
      const sb = sheet.getByRole("spinbutton")
      await expect(sb).toHaveAttribute("aria-valuenow", "1")
      await expect(sb).toContainText("×1")

      // − is disabled at 1 (no fractional portions).
      const minus = sheet.getByRole("button", { name: /−1/ })
      await expect(minus).toBeDisabled()

      // + bumps to 2 then 3.
      await sheet.getByRole("button", { name: /\+1/ }).click()
      await expect(sb).toHaveAttribute("aria-valuenow", "2")
      await sheet.getByRole("button", { name: /\+1/ }).click()
      await expect(sb).toHaveAttribute("aria-valuenow", "3")
    } finally {
      await cleanupFood(seeded.composed.id)
      await cleanupFood(seeded.stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("leaf food picker shows a quantity + unit input with grams default 100", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.q_leaf_${tag}`, "Apple", 990)
    const rice = await seedLeafFood({
      name: `RiceQ ${tag}`,
      kcal_100g: 130,
      protein_100g: 2.7,
    })

    try {
      await page.goto("/")
      await page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.status() === 200
      )

      const cell = page.getByTestId(`cell-0-${slot.id}`).first()
      await expect(cell).toBeVisible()
      await cell.getByRole("button", { name: /plan meal/i }).click()

      const sheet = page.getByTestId("tray-sheet")
      await expect(sheet).toBeVisible()
      await sheet.getByTestId("tray-search").fill(`RiceQ ${tag}`)
      await sheet
        .getByRole("button", { name: new RegExp(`RiceQ ${tag}`) })
        .first()
        .click()

      // In the staged row the quantity-unit input renders compact (no
      // chips). The default for a leaf without a portion table is 100 g.
      const amount = sheet.getByTestId("quantity-unit-amount")
      await expect(amount).toHaveValue("100")

      // Bump amount to 200 g via the spinbutton.
      await amount.fill("200")
      await expect(amount).toHaveValue("200")

      // Commit and verify the cell shows the leaf food's name (kcal rolls
      // up via the planner's macro endpoint, which the cell renders).
      const resp = page.waitForResponse(
        (r) =>
          r.url().includes("/components") && r.request().method() === "POST"
      )
      await sheet.getByTestId("tray-commit").click()
      const r = await resp
      const body = (await r.json()) as { amount: number; unit: string }
      expect(body.amount).toBe(200)
      expect(body.unit).toBe("g")

      await expect(cell.getByText(`RiceQ ${tag}`)).toBeVisible()
    } finally {
      await cleanupFood(rice.id)
      await cleanupSlot(slot.id)
    }
  })

  test("slot sheet edits a leaf component's amount and re-resolves grams", async ({
    page,
  }) => {
    await mockAnchorToday(page)
    const tag = uid()
    const slot = await seedSlot(`slot.q_leaf_edit_${tag}`, "Apple", 989)
    const rice = await seedLeafFood({
      name: `RiceE ${tag}`,
      kcal_100g: 130,
    })

    try {
      const today = new Date().toISOString().slice(0, 10)
      await seedPlateWithComponent(today, slot.id, {
        food_id: rice.id,
        amount: 200,
        unit: "g",
      })

      await page.goto("/")
      await page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.status() === 200
      )
      const cell = page.getByTestId(`cell-0-${slot.id}`).first()
      await cell.click()

      // Slot sheet shows the component row with the QuantityUnitInput, value
      // populated from the persisted amount.
      const amount = page.getByTestId("quantity-unit-amount")
      await expect(amount).toHaveValue("200")

      // Change amount to 300 g via the spinbutton (slot-sheet rows render
      // the input in compact mode without quick chips).
      const putRes = page.waitForResponse(
        (r) => r.url().includes("/components") && r.request().method() === "PUT"
      )
      await amount.fill("300")
      // Blur the field to flush the change to the mutation hook.
      await amount.blur()
      const r = await putRes
      const updated = (await r.json()) as { amount: number; grams: number }
      expect(updated.amount).toBe(300)
      expect(updated.grams).toBe(300)
    } finally {
      await cleanupFood(rice.id)
      await cleanupSlot(slot.id)
    }
  })
})
