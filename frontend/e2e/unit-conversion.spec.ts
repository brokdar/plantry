import { test, expect, apiRequest } from "./helpers"

import { API, cleanupIngredient, seedIngredient, uid } from "./helpers"

async function seedPortion(ingredientId: number, unit: string, grams: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post(`/api/ingredients/${ingredientId}/portions`, {
    data: { unit, grams },
  })
  expect(
    res.ok(),
    `Seed portion ${unit} for ${ingredientId} failed: ${res.status()}`
  ).toBeTruthy()
  await ctx.dispose()
}

async function pickIngredient(
  page: import("@playwright/test").Page,
  rowIndex: number,
  ingredientId: number,
  searchTerm: string
) {
  const comboboxId = `ingredient-row-${rowIndex}-combobox`
  await page.getByTestId(comboboxId).click()
  await page.getByTestId(`${comboboxId}-search`).fill(searchTerm)
  await page.getByTestId(`${comboboxId}-option-${ingredientId}`).click()
}

async function setUnit(
  page: import("@playwright/test").Page,
  rowIndex: number,
  unit: string
) {
  await page.getByTestId(`ingredient-row-${rowIndex}-unit`).click()
  await page.getByTestId(`unit-option-${unit}`).click()
}

async function setAmount(
  page: import("@playwright/test").Page,
  rowIndex: number,
  value: string
) {
  const row = page.getByTestId(`ingredient-row-${rowIndex}`)
  // amount is the first numeric input in the row (order: amount, grams).
  await row.locator('input[type="number"]').first().fill(value)
}

test.describe("Unit conversion", () => {
  test("honey tbsp uses the ingredient-specific portion", async ({ page }) => {
    const tag = uid()
    const ing = await seedIngredient({
      name: `E2E Honey ${tag}`,
      kcal_100g: 304,
      carbs_100g: 82,
    })
    await seedPortion(ing.id, "tbsp", 21)

    try {
      await page.goto("/components/new")
      await page.getByLabel(/^name/i).fill(`Sauce ${tag}`)
      await page.getByTestId("add-ingredient").click()

      const row = page.getByTestId("ingredient-row-0")
      await expect(row).toBeVisible()

      await pickIngredient(page, 0, ing.id, `Honey ${tag}`)
      await setAmount(page, 0, "2")
      await setUnit(page, 0, "tbsp")

      const grams = page.getByTestId("ingredient-row-0-grams")
      await expect(grams).toHaveValue("42")

      const badge = page.getByTestId("ingredient-row-0-badge")
      await expect(badge).toHaveAttribute("data-source", "portion")
    } finally {
      await cleanupIngredient(ing.id)
    }
  })

  test("tbsp without portion falls back to water-density", async ({ page }) => {
    const tag = uid()
    const ing = await seedIngredient({ name: `E2E Flour ${tag}` })

    try {
      await page.goto("/components/new")
      await page.getByLabel(/^name/i).fill(`Bake ${tag}`)
      await page.getByTestId("add-ingredient").click()

      await pickIngredient(page, 0, ing.id, `Flour ${tag}`)
      await setAmount(page, 0, "2")
      await setUnit(page, 0, "tbsp")

      const grams = page.getByTestId("ingredient-row-0-grams")
      await expect(grams).toHaveValue("30")

      const badge = page.getByTestId("ingredient-row-0-badge")
      await expect(badge).toHaveAttribute("data-source", "fallback")
    } finally {
      await cleanupIngredient(ing.id)
    }
  })

  test("count unit without portion can be resolved via inline Add portion", async ({
    page,
  }) => {
    const tag = uid()
    const ing = await seedIngredient({ name: `E2E Garlic ${tag}` })

    try {
      await page.goto("/components/new")
      await page.getByLabel(/^name/i).fill(`Dish ${tag}`)
      await page.getByTestId("add-ingredient").click()

      await pickIngredient(page, 0, ing.id, `Garlic ${tag}`)
      await setAmount(page, 0, "2")
      await setUnit(page, 0, "clove")

      const badge = page.getByTestId("ingredient-row-0-badge")
      await expect(badge).toHaveAttribute("data-source", "unresolved")

      await page.getByTestId("ingredient-row-0-add-portion").click()
      await page.getByTestId("ingredient-row-0-add-portion-grams").fill("4")

      const portionResponse = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/ingredients/${ing.id}/portions`) &&
          res.request().method() === "POST"
      )
      await page.getByTestId("ingredient-row-0-save-portion").click()
      await portionResponse

      await expect(badge).toHaveAttribute("data-source", "portion")
      await expect(page.getByTestId("ingredient-row-0-grams")).toHaveValue("8")
    } finally {
      await cleanupIngredient(ing.id)
    }
  })
})
