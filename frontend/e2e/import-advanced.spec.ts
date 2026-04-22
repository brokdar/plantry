import { expect, test } from "./helpers"

test.describe("Recipe import — error paths", () => {
  test("malformed HTML paste shows a helpful inline error", async ({
    page,
  }) => {
    await page.goto("/import")

    // Switch to the HTML paste panel.
    await page
      .getByRole("button", { name: /paste the page html instead/i })
      .click()

    // Paste HTML that does not contain a recipe.
    await page
      .getByLabel(/page html/i)
      .fill("<html><body><p>not a recipe</p></body></html>")

    const resp = page.waitForResponse(
      (r) =>
        r.url().includes("/api/import/extract") &&
        r.request().method() === "POST"
    )
    await page.getByRole("button", { name: /extract recipe/i }).click()
    const res = await resp
    expect(res.status()).toBeGreaterThanOrEqual(400)

    // The inline submit error surfaces an i18n message from the import
    // error family — either "No recipe was found" or "AI could not extract",
    // depending on whether the backend short-circuits on JSON-LD or tries
    // the LLM path first.
    await expect(
      page.getByText(/No recipe was found|AI could not extract/i)
    ).toBeVisible()
  })
})
