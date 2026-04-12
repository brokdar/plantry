import { test, expect } from "@playwright/test"

test.describe("Ingredient Resolution", () => {
  test("create ingredient from lookup", async ({ page }) => {
    await page.route("**/api/ingredients/lookup*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          results: [
            {
              name: "Chicken Breast, Raw",
              source: "fdc",
              barcode: null,
              fdc_id: 171077,
              image_url: null,
              existing_id: null,
              kcal_100g: 120,
              protein_100g: 22.5,
              fat_100g: 2.6,
              carbs_100g: 0,
              fiber_100g: 0,
              sodium_100g: 0.074,
              portions: [],
            },
          ],
          recommended_index: 0,
        }),
      })
    })

    await page.goto("/ingredients/new")

    // Should show search tab by default
    const searchInput = page.getByPlaceholder(/search by name or barcode/i)
    await searchInput.fill("chicken breast")

    // Wait for candidate to appear
    await expect(page.getByText("Chicken Breast, Raw")).toBeVisible()

    // Click candidate
    await page.getByText("Chicken Breast, Raw").click()

    // Form should be populated
    await expect(page.getByLabel(/name/i)).toHaveValue("Chicken Breast, Raw")

    // Save
    const savePromise = page.waitForResponse(
      (res) =>
        res.url().includes("/api/ingredients") &&
        res.request().method() === "POST"
    )
    await page.getByRole("button", { name: /save/i }).click()
    await savePromise

    // Should navigate to ingredients list
    await expect(page).toHaveURL(/\/ingredients/)
  })

  test("switch to manual tab and create ingredient", async ({ page }) => {
    await page.goto("/ingredients/new")

    // Click manual tab
    await page.getByRole("tab", { name: /manual/i }).click()

    // Fill in form
    const uid = crypto.randomUUID().slice(0, 8)
    const name = `Manual ingredient ${uid}`
    await page.getByLabel(/name/i).fill(name)
    await page.getByLabel(/calories/i).fill("100")

    // Save
    const savePromise = page.waitForResponse(
      (res) =>
        res.url().includes("/api/ingredients") &&
        res.request().method() === "POST"
    )
    await page.getByRole("button", { name: /save/i }).click()
    await savePromise

    // Should navigate to ingredients list
    await expect(page).toHaveURL(/\/ingredients/)
  })
})
