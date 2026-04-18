import { expect, test } from "./helpers"

import {
  cleanupComponent,
  cleanupSlot,
  cleanupTemplate,
  seedComponent,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Templates", () => {
  test("save a plate as a template, then apply it to an empty cell", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 999)
    const main = await seedComponent({
      name: `Chicken curry ${tag}`,
      role: "main",
    })
    const side = await seedComponent({
      name: `Basmati ${tag}`,
      role: "side_starch",
    })

    let templateId: number | undefined

    try {
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()

      // Create a plate with the main on Monday.
      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /add a meal/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Chicken curry ${tag}`) })
        .click()
      await createPlateResp

      // Add a side.
      const addCompResp = page.waitForResponse(
        (r) => /\/components$/.test(r.url()) && r.request().method() === "POST"
      )
      await cell
        .getByRole("button", { name: /add component/i })
        .first()
        .click()
      await page
        .getByRole("button", { name: new RegExp(`Basmati ${tag}`) })
        .click()
      await addCompResp

      await expect(cell.getByText(`Basmati ${tag}`)).toBeVisible()

      // Wait for the Add sheet overlay to fully detach before clicking Actions.
      await expect(page.getByRole("dialog")).toHaveCount(0)

      // Save as template.
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
      await cell.getByRole("button", { name: /actions/i }).click()
      const deletePlateResp = page.waitForResponse(
        (r) =>
          /\/plates\/\d+$/.test(r.url()) && r.request().method() === "DELETE"
      )
      await page.getByRole("menuitem", { name: /delete plate/i }).click()
      await deletePlateResp

      await expect(cell.getByText(`Chicken curry ${tag}`)).toHaveCount(0)

      // Reopen the sheet from the empty cell and apply the template.
      await expect(page.getByRole("dialog")).toHaveCount(0)
      await cell.getByRole("button", { name: /add a meal/i }).click()

      const createEmptyPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      const applyResp = page.waitForResponse(
        (r) =>
          /\/api\/templates\/\d+\/apply$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await page.getByTestId(`apply-template-${templateId}`).click()
      await createEmptyPlateResp
      await applyResp

      // Both components from the template should reappear.
      await expect(cell.getByText(`Chicken curry ${tag}`)).toBeVisible()
      await expect(cell.getByText(`Basmati ${tag}`)).toBeVisible()
    } finally {
      if (templateId !== undefined) await cleanupTemplate(templateId)
      await cleanupComponent(main.id)
      await cleanupComponent(side.id)
      await cleanupSlot(slot.id)
    }
  })

  test("empty state on /templates page", async ({ page }) => {
    await page.goto("/templates")
    const grid = page.getByTestId("template-grid")
    const empty = page.getByTestId("template-empty")
    // Either the grid has items or the empty state shows.
    const hasGrid = (await grid.count()) > 0
    if (!hasGrid) {
      await expect(empty).toBeVisible()
    }
  })
})
