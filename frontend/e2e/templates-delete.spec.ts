import { cleanupTemplate, expect, seedTemplate, test, uid } from "./helpers"

test.describe("Template deletion", () => {
  test("delete a template from the grid", async ({ page }) => {
    const tag = uid()
    const tpl = await seedTemplate({ name: `Tpl Delete ${tag}` })

    try {
      await page.goto("/templates")
      const card = page.getByTestId(`template-card-${tpl.id}`)
      await expect(card).toBeVisible()

      await card.getByRole("button", { name: /delete/i }).click()

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/templates/${tpl.id}`) &&
          r.request().method() === "DELETE"
      )
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click()
      await resp

      await expect(page.getByTestId(`template-card-${tpl.id}`)).toHaveCount(0)
    } finally {
      // Best-effort in case deletion did not actually happen.
      await cleanupTemplate(tpl.id)
    }
  })
})
