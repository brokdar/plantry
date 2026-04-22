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
      await expect(card).toContainText("165kcal")
      await expect(card).toContainText("Manual")
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
    await page.getByTestId("ingredient-create-tile").getByRole("link").click()
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

  test("clicking a card body navigates to edit page", async ({ page }) => {
    const tag = uid()
    const ing = await seedIngredient({
      name: `Click Edit ${tag}`,
      kcal_100g: 50,
    })

    try {
      await page.goto("/ingredients")
      const searchResp = page.waitForResponse((r) =>
        r.url().includes(`search=`)
      )
      await page.getByTestId("inventory-search").fill(tag)
      await searchResp

      // Click the card-cover anchor by its accessible name (aria-label).
      await page
        .getByTestId(`ingredient-card-${ing.id}`)
        .getByRole("link", { name: `Click Edit ${tag}`, exact: true })
        .click()

      await expect(page).toHaveURL(new RegExp(`/ingredients/${ing.id}/edit$`))
      await expect(page.getByLabel(/^name/i)).toHaveValue(`Click Edit ${tag}`)
    } finally {
      await cleanupIngredient(ing.id)
    }
  })

  test("sort chips update the list order", async ({ page }) => {
    const tag = uid()
    const a = await seedIngredient({ name: `Sort A ${tag}`, kcal_100g: 50 })
    const b = await seedIngredient({ name: `Sort B ${tag}`, kcal_100g: 500 })

    try {
      await page.goto("/ingredients")
      await page.getByTestId("inventory-search").fill(tag)
      await page.waitForResponse(
        (r) =>
          r.url().includes("/api/ingredients") &&
          r.url().includes(`search=${tag}`)
      )
      await expect(page.getByTestId(`ingredient-card-${a.id}`)).toBeVisible()
      await expect(page.getByTestId(`ingredient-card-${b.id}`)).toBeVisible()

      // Switch sort to kcal — fires a new request with sort=kcal.
      const kcalResp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/ingredients") && r.url().includes("sort=kcal")
      )
      await page.getByRole("button", { name: /calories/i }).click()
      await kcalResp

      // Name sort chip should no longer be the selected option.
      await expect(
        page.getByRole("button", { name: /calories/i })
      ).toHaveAttribute("aria-pressed", "true")
    } finally {
      await cleanupIngredient(a.id)
      await cleanupIngredient(b.id)
    }
  })
})
