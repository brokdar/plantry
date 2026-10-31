import { cleanupFood, expect, seedLeafFood, test, uid } from "./helpers"

test.describe("Ingredient Catalogue", () => {
  test("create an ingredient via the form", async ({ page }) => {
    const tag = uid()
    const name = `Chicken breast ${tag}`
    let createdId: number | undefined

    try {
      await page.goto("/recipes/ingredients/new")
      await page.getByLabel(/^name/i).fill(name)
      await page.getByLabel(/calories/i).fill("165")
      await page.getByLabel(/protein/i).fill("31")
      await page.getByLabel(/^fat/i).fill("3.6")
      await page.getByLabel(/^carbs/i).fill("0")

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") && res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(201)

      const body = (await response.json()) as { id: number }
      createdId = body.id

      // Back on the table, the new row shows its kcal.
      await expect(page).toHaveURL(/\/recipes\/ingredients$/)
      await page.getByTestId("ingredients-search").fill(tag)
      await expect(
        page.getByTestId(`ingredient-row-${createdId}`)
      ).toContainText("165")
    } finally {
      if (createdId) await cleanupFood(createdId)
    }
  })

  test("search filters ingredients by name", async ({ page }) => {
    const tag = uid()
    const chicken = await seedLeafFood({
      name: `Chicken thigh ${tag}`,
      kcal_100g: 209,
    })
    const tofu = await seedLeafFood({
      name: `Tofu ${tag}`,
      kcal_100g: 76,
    })
    const rice = await seedLeafFood({
      name: `Basmati rice ${tag}`,
      kcal_100g: 130,
    })

    try {
      await page.goto("/recipes/ingredients")

      // Narrow to seeded items only — shared DB accumulates across runs.
      await page.getByTestId("ingredients-search").fill(tag)
      await page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") &&
          res.url().includes(`search=${tag}`)
      )

      // All three seeded items visible
      await expect(
        page.getByTestId(`ingredient-row-${chicken.id}`)
      ).toBeVisible()
      await expect(page.getByTestId(`ingredient-row-${tofu.id}`)).toBeVisible()
      await expect(page.getByTestId(`ingredient-row-${rice.id}`)).toBeVisible()

      // Search — only chicken matches
      await page.getByTestId("ingredients-search").fill(`chicken ${tag}`)
      await page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") &&
          res.url().includes("chicken") &&
          res.url().includes(tag)
      )

      await expect(
        page.getByTestId(`ingredient-row-${chicken.id}`)
      ).toBeVisible()
      await expect(page.getByTestId(`ingredient-row-${tofu.id}`)).toHaveCount(0)
      await expect(page.getByTestId(`ingredient-row-${rice.id}`)).toHaveCount(0)
    } finally {
      await cleanupFood(chicken.id)
      await cleanupFood(tofu.id)
      await cleanupFood(rice.id)
    }
  })

  test("edit an ingredient", async ({ page }) => {
    const name = `Brown rice ${uid()}`
    const ingredient = await seedLeafFood({ name, kcal_100g: 112 })

    try {
      await page.goto(`/recipes/ingredients/${ingredient.id}/edit`)
      await expect(page.getByLabel(/^name/i)).toHaveValue(name)

      // Change kcal
      await page.getByLabel(/calories/i).fill("120")

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/foods/${ingredient.id}`) &&
          res.request().method() === "PUT"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(200)

      // Back on the table, the row shows the new value.
      await expect(page).toHaveURL(/\/recipes\/ingredients$/)
      await page.getByTestId("ingredients-search").fill(name)
      await expect(
        page.getByTestId(`ingredient-row-${ingredient.id}`)
      ).toContainText("120")
    } finally {
      await cleanupFood(ingredient.id)
    }
  })

  test("delete an ingredient from its editor", async ({ page }) => {
    const toDelete = await seedLeafFood({
      name: `Butter ${uid()}`,
      kcal_100g: 717,
    })

    await page.goto(`/recipes/ingredients/${toDelete.id}/edit`)
    await page.getByTestId("ingredient-delete").click()
    const responsePromise = page.waitForResponse(
      (res) =>
        res.url().includes(`/api/foods/${toDelete.id}`) &&
        res.request().method() === "DELETE"
    )
    await page.getByRole("dialog").getByTestId("confirm-delete").click()
    expect((await responsePromise).ok()).toBeTruthy()
    await expect(page).toHaveURL(/\/recipes\/ingredients$/)

    await page.getByTestId("ingredients-search").fill(toDelete.name)
    await expect(page.getByTestId("ingredients-empty")).toBeVisible()
  })

  test("shows validation error when submitting empty name", async ({
    page,
  }) => {
    await page.goto("/recipes/ingredients/new")

    // Fill a macro field but leave name empty
    await page.getByLabel(/calories/i).fill("100")

    // Save button is disabled until the name field is non-empty — this is
    // the client-side guard against submitting an empty name.
    const save = page.getByRole("button", { name: /save/i })
    await expect(save).toBeDisabled()

    // No cleanup needed — nothing was created
  })

  test("shows error when creating ingredient with duplicate name", async ({
    page,
  }) => {
    const name = `Duplicate test ${uid()}`
    const existing = await seedLeafFood({ name, kcal_100g: 100 })

    try {
      await page.goto("/recipes/ingredients/new")
      await page.getByLabel(/^name/i).fill(name)
      await page.getByLabel(/calories/i).fill("200")

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") && res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(409)

      // Backend returns 409 with message_key error.food.duplicate_name; the
      // form surfaces the translated message (or the key if untranslated).
      await expect(
        page.locator("#main").getByText(/already exists|food\.duplicate_name/i)
      ).toBeVisible()
    } finally {
      await cleanupFood(existing.id)
    }
  })
})
