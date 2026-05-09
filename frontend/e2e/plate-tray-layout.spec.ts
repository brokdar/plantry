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
// `seedComposedFood` is used to seed the existing-plate dish in the
// "non-empty plate" test below.

/** Seed a plate via the date-keyed POST endpoint and add one component so
 *  the tray opens on a non-empty plate. Returns the plate id for cleanup
 *  via the slot teardown helpers. */
async function seedPlateWithComponent(
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

function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

// Phase 4 of the plate workflow rework — composition-first tray layout.
// The picker now anchors on what the user is building: existing plate
// components are shown muted under the staged additions, with a single
// running total + macro bar at the top of the sheet.
test.describe("Plate tray layout — Phase 4 composition-first preview", () => {
  test("empty slot: preview renders the empty state, no running kcal", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 800)
    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()
      await cell.getByRole("button", { name: /plan meal/i }).click()

      const sheet = page.getByTestId("tray-sheet")
      await expect(sheet).toBeVisible()
      // Empty state copy renders, running-total kcal is suppressed.
      await expect(sheet.getByTestId("draft-plate-preview-empty")).toBeVisible()
      await expect(sheet.getByTestId("tray-running-kcal")).toHaveCount(0)
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("staging items: preview shows pills, hero, and a running total above the search", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 800)
    const tofu = await seedLeafFood({ name: `Tofu ${tag}`, kcal_100g: 80 })
    const rice = await seedLeafFood({ name: `Rice ${tag}`, kcal_100g: 130 })
    const broccoli = await seedLeafFood({
      name: `Broccoli ${tag}`,
      kcal_100g: 35,
    })
    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet = page.getByTestId("tray-sheet")
      await expect(sheet).toBeVisible()

      // Stage three items and verify each shows up as a staged pill.
      for (const name of [`Tofu ${tag}`, `Rice ${tag}`, `Broccoli ${tag}`]) {
        await sheet.locator("input").first().fill(name)
        await sheet
          .getByRole("button", { name: new RegExp(name) })
          .first()
          .click()
        await sheet.locator("input").first().fill("")
      }

      const preview = sheet.getByTestId("draft-plate-preview")
      await expect(preview).toBeVisible()
      // Hero collage renders for non-empty plate.
      await expect(sheet.getByTestId("draft-plate-preview-hero")).toBeVisible()
      // Three staged pills in the preview pills row.
      await expect(
        sheet
          .getByTestId("draft-plate-preview-pills")
          .locator("[data-state='staged']")
      ).toHaveCount(3)
      // Running total renders with a non-zero kcal value.
      await expect(sheet.getByTestId("tray-running-kcal")).toBeVisible()
      await expect(sheet.getByTestId("tray-running-kcal")).not.toContainText(
        "0 kcal"
      )

      // Running total chip should not double-render — it's only inside the
      // preview now, never duplicated in the footer.
      await expect(sheet.getByTestId("tray-running-kcal")).toHaveCount(1)
      await expect(sheet.getByTestId("tray-running-total")).toHaveCount(0)
    } finally {
      await cleanupFood(tofu.id)
      await cleanupFood(rice.id)
      await cleanupFood(broccoli.id)
      await cleanupSlot(slot.id)
    }
  })

  test("non-empty plate: preview shows existing components muted under staged additions", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 800)
    const stub = await seedLeafFood({ name: `Stub ${tag}`, kcal_100g: 100 })
    const existingDish = await seedComposedFood({
      name: `Stew ${tag}`,
      role: "main",
      reference_portions: 1,
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
    const newSide = await seedLeafFood({
      name: `Salad ${tag}`,
      kcal_100g: 20,
    })

    // Seed a plate that already has the existing dish on it so the tray
    // opens on a non-empty plate and the preview can render the muted
    // existing-vs-staged distinction.
    await seedPlateWithComponent(todayISO(), slot.id, existingDish.id)

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()
      // Open the slot sheet first (the cell is filled), then click the
      // "Add component" affordance which routes through the same tray.
      await cell.getByTestId("slot-open-sheet").click()
      const slotSheet = page.getByTestId("slot-sheet")
      await expect(slotSheet).toBeVisible()
      await slotSheet.getByTestId("slot-sheet-add-component").click()

      const sheet = page.getByTestId("tray-sheet")
      await expect(sheet).toBeVisible()

      // Preview renders, with at least one existing pill and zero staged
      // pills before the user touches anything.
      await expect(
        sheet
          .getByTestId("draft-plate-preview-pills")
          .locator("[data-state='existing']")
      ).toHaveCount(1)
      await expect(
        sheet
          .getByTestId("draft-plate-preview-pills")
          .locator("[data-state='staged']")
      ).toHaveCount(0)

      // Stage the new side; the existing pill stays, a staged pill is added
      // alongside it. This is the "what am I editing?" assertion: the user
      // can see both layers at once.
      await sheet.locator("input").first().fill(`Salad ${tag}`)
      await sheet
        .getByRole("button", { name: new RegExp(`Salad ${tag}`) })
        .first()
        .click()
      await expect(
        sheet
          .getByTestId("draft-plate-preview-pills")
          .locator("[data-state='existing']")
      ).toHaveCount(1)
      await expect(
        sheet
          .getByTestId("draft-plate-preview-pills")
          .locator("[data-state='staged']")
      ).toHaveCount(1)
    } finally {
      await cleanupFood(newSide.id)
      await cleanupFood(existingDish.id)
      await cleanupFood(stub.id)
      // cleanupSlot also tears down any plates that reference the slot.
      await cleanupSlot(slot.id)
    }
  })

  test.describe("mobile bottom sheet", () => {
    test.use({ viewport: { width: 390, height: 844 }, isMobile: true })

    test("preview is collapsed by default on mobile, expands on tap", async ({
      page,
    }) => {
      const tag = uid()
      const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 800)
      const dish = await seedLeafFood({
        name: `Tofu ${tag}`,
        kcal_100g: 80,
      })
      try {
        await page.goto("/")
        // Mobile planner uses a different cell testid prefix.
        const cell = page
          .locator(`[data-testid="mobile-cell-0-${slot.id}"]`)
          .first()
        await expect(cell).toBeVisible()
        await cell.getByRole("button", { name: /plan meal/i }).click()
        const sheet = page.getByTestId("tray-sheet")
        await expect(sheet).toBeVisible()

        // Stage one item so the running total is non-zero — this matters
        // because the collapsed summary still surfaces kcal.
        await sheet.locator("input").first().fill(`Tofu ${tag}`)
        await sheet
          .getByRole("button", { name: new RegExp(`Tofu ${tag}`) })
          .first()
          .click()

        // Collapsed summary visible by default; full preview not in DOM.
        const collapsed = sheet.getByTestId("draft-plate-preview-collapsed")
        await expect(collapsed).toBeVisible()
        await expect(sheet.getByTestId("draft-plate-preview")).toHaveCount(0)
        // kcal is still surfaced inside the collapsed shell.
        await expect(sheet.getByTestId("tray-running-kcal")).toBeVisible()

        await collapsed.click()
        await expect(sheet.getByTestId("draft-plate-preview")).toBeVisible()
        await expect(
          sheet.getByTestId("draft-plate-preview-collapsed")
        ).toHaveCount(0)
      } finally {
        await cleanupFood(dish.id)
        await cleanupSlot(slot.id)
      }
    })
  })
})
