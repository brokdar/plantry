import { expect, test } from "./helpers"

import {
  cleanupFood,
  cleanupSlot,
  cleanupTemplate,
  pickAndCommitFood,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Templates", () => {
  test("save a plate as a template, then apply it to an empty cell", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 999)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const main = await seedComposedFood({
      name: `Chicken curry ${tag}`,
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

    let templateId: number | undefined

    try {
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()

      // Create a plate via the picker sheet.
      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet = page.getByRole("dialog")
      await expect(sheet).toBeVisible()
      await sheet.locator("input").first().fill(`Chicken curry ${tag}`)
      await pickAndCommitFood(page, sheet, new RegExp(`Chicken curry ${tag}`))
      await expect(cell.getByText(`Chicken curry ${tag}`)).toBeVisible()

      // Save as template.
      await cell.hover()
      await cell.getByRole("button", { name: /actions/i }).click()
      await page.getByRole("menuitem", { name: /save as template/i }).click()

      const createTplResp = page.waitForResponse(
        (r) =>
          /\/api\/templates$/.test(r.url()) && r.request().method() === "POST"
      )
      await page.getByLabel(/template name/i).fill(`Template ${tag}`)
      await page.getByRole("button", { name: /create template/i }).click()
      const created = await createTplResp
      expect(created.status()).toBe(201)
      templateId = ((await created.json()) as { id: number }).id

      // Delete the plate so the cell is empty again.
      await cell.hover()
      await cell.getByRole("button", { name: /actions/i }).click()
      const deletePlateResp = page.waitForResponse(
        (r) =>
          /\/plates\/\d+$/.test(r.url()) && r.request().method() === "DELETE"
      )
      await page.getByRole("menuitem", { name: /delete plate/i }).click()
      await deletePlateResp
      await expect(cell.getByText(`Chicken curry ${tag}`)).toHaveCount(0)

      // Apply the saved slot-scope template via the slot-row overflow
      // menu. The redesigned planner exposes apply for slot templates
      // through the row menu (RowApplyTemplateDialog), not the per-day
      // header menu — that one is scoped to "day"-scope templates only.
      await page.getByTestId(`slot-row-${slot.id}`).first().hover()
      await page.getByTestId(`slot-row-menu-${slot.id}`).click()
      await page.getByTestId(`slot-row-apply-${slot.id}`).click()

      const tplDialog = page.getByRole("dialog")
      await expect(tplDialog).toBeVisible()
      await tplDialog.getByTestId(`row-apply-item-${templateId}`).click()

      const applyResp = page.waitForResponse(
        (r) =>
          /\/api\/templates\/\d+\/apply$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await tplDialog.getByTestId("row-apply-submit").click()
      const applied = await applyResp
      expect(applied.ok()).toBe(true)
    } finally {
      if (templateId !== undefined) await cleanupTemplate(templateId)
      await cleanupFood(main.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("empty state on /templates page", async ({ page }) => {
    await page.goto("/templates")
    const grid = page.getByTestId("template-grid")
    const empty = page.getByTestId("template-empty")
    // Either the grid has items or the empty state shows. Wait until one of
    // them is mounted so the assertion isn't racing the initial query.
    await expect(grid.or(empty)).toBeVisible()
    if ((await grid.count()) === 0) {
      await expect(empty).toBeVisible()
    }
  })
})
