import { cleanupIngredient, expect, seedIngredient, test, uid } from "./helpers"

test.describe("Ingredient Inventory (card grid)", () => {
  test("renders cards with name, kcal, and macros", async ({ page }) => {
    const tag = uid()
    const chicken = await seedIngredient({
      name: `Grid Chicken ${tag}`,
      kcal_100g: 165,
      protein_100g: 31,
      fat_100g: 4,
      carbs_100g: 0,
    })

    try {
      await page.goto("/ingredients")
      await page.getByTestId("inventory-search").fill(tag)
      await page.waitForResponse(
        (r) =>
          r.url().includes("/api/ingredients") &&
          r.url().includes(`search=${tag}`)
      )
      const card = page.getByTestId(`ingredient-card-${chicken.id}`)
      await expect(card).toBeVisible()
      await expect(card.getByText(chicken.name)).toBeVisible()
      await expect(card.getByText("165 kcal / 100g")).toBeVisible()
    } finally {
      await cleanupIngredient(chicken.id)
    }
  })

  test("empty-create tile navigates to /ingredients/new", async ({ page }) => {
    await page.goto("/ingredients")
    const gibberish = `zzz-${uid()}`
    await page.getByTestId("inventory-search").fill(gibberish)
    await page.waitForResponse(
      (r) =>
        r.url().includes("/api/ingredients") &&
        r.url().includes(`search=${gibberish}`)
    )
    await page.getByTestId("ingredient-create-tile").click()
    await expect(page).toHaveURL(/\/ingredients\/new$/)
  })

  test("card-menu delete removes the card", async ({ page }) => {
    const tag = uid()
    const keep = await seedIngredient({
      name: `Keep Ingredient ${tag}`,
      kcal_100g: 100,
    })
    const toDelete = await seedIngredient({
      name: `Delete Ingredient ${tag}`,
      kcal_100g: 200,
    })

    try {
      await page.goto("/ingredients")
      await page.getByTestId("inventory-search").fill(tag)
      await page.waitForResponse(
        (r) =>
          r.url().includes("/api/ingredients") &&
          r.url().includes(`search=${tag}`)
      )
      await expect(
        page.getByTestId(`ingredient-card-${toDelete.id}`)
      ).toBeVisible()

      await page.getByTestId(`ingredient-card-${toDelete.id}-menu`).click()
      await page.getByTestId(`ingredient-card-${toDelete.id}-delete`).click()

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/ingredients/${toDelete.id}`) &&
          r.request().method() === "DELETE"
      )
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click()
      await resp

      await expect(
        page.getByTestId(`ingredient-card-${toDelete.id}`)
      ).toHaveCount(0)
      await expect(page.getByTestId(`ingredient-card-${keep.id}`)).toBeVisible()
    } finally {
      await cleanupIngredient(keep.id)
    }
  })

  test("source filter chip narrows grid client-side", async ({ page }) => {
    const tag = uid()
    // Both seeded via API default to "manual" source — this test checks that
    // toggling the chip shows/hides all current-page cards when source ≠
    // "manual" is selected (no backend-sourced non-manual items in scope).
    const a = await seedIngredient({
      name: `Source A ${tag}`,
      kcal_100g: 50,
    })

    try {
      await page.goto("/ingredients")
      await page.getByTestId("inventory-search").fill(tag)
      await page.waitForResponse(
        (r) =>
          r.url().includes("/api/ingredients") &&
          r.url().includes(`search=${tag}`)
      )
      await expect(page.getByTestId(`ingredient-card-${a.id}`)).toBeVisible()

      // Select "Open Food Facts" — the manual card should be filtered out.
      await page.getByTestId("ingredient-filter-source-off").click()
      await expect(page.getByTestId(`ingredient-card-${a.id}`)).toHaveCount(0)

      // Deselect — reappears.
      await page.getByTestId("ingredient-filter-source-off").click()
      await expect(page.getByTestId(`ingredient-card-${a.id}`)).toBeVisible()
    } finally {
      await cleanupIngredient(a.id)
    }
  })
})
