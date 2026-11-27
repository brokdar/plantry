import {
  cleanupFood,
  expect,
  seedComposedFood,
  seedLeafFood,
  test,
  uid,
} from "./helpers"

// B2b: legacy recipes with a nutritionless ingredient wait under "Needs your
// check" (a quiet count line, never a banner) until the ingredient is fixed.
test("a legacy recipe appears under Needs your check and leaves it once its ingredient has nutrition", async ({
  page,
}) => {
  const tag = uid()
  const rice = await seedLeafFood({
    name: `E2E Reis ${tag}`,
    kcal_100g: 130,
    protein_100g: 3,
    fat_100g: 0.3,
    carbs_100g: 28,
  })
  const legacy = await seedLeafFood({ name: `E2E Altlast ${tag}` })
  const recipe = await seedComposedFood({
    name: `E2E Legacy ${tag}`,
    role: "main",
    children: [
      { child_id: rice.id, amount: 100, unit: "g", grams: 100, sort_order: 0 },
      {
        child_id: legacy.id,
        amount: 50,
        unit: "g",
        grams: 50,
        sort_order: 1,
      },
    ],
  })

  try {
    await page.goto("/recipes")
    await expect(page.getByTestId("needs-check-entry")).toBeVisible()
    // The card says its nutrition is incomplete.
    await expect(
      page.getByTestId(`recipe-card-${recipe.id}-meta`)
    ).toContainText(/incomplete/)

    await page.getByTestId("needs-check-entry").click()
    const list = page.getByTestId("needs-check-list")
    await expect(list.getByRole("link", { name: recipe.name })).toBeVisible()
    await expect(list.getByText(`E2E Altlast ${tag}`)).toBeVisible()

    // Other specs may leave their own legacy rows: work in this recipe's.
    const mine = list
      .getByRole("listitem")
      .filter({ has: page.getByRole("link", { name: recipe.name }) })
    await mine.getByRole("button", { name: "Enter nutrition" }).click()
    await page.getByLabel("kcal", { exact: true }).fill("90")
    await page.getByLabel("Protein (g)").fill("2")
    await page.getByLabel("Fat (g)").fill("1")
    await page.getByLabel("Carbs (g)").fill("15")
    await page.getByRole("button", { name: "Save nutrition" }).click()

    await expect(list.getByRole("link", { name: recipe.name })).toHaveCount(0)
    await page.keyboard.press("Escape")
    await expect(
      page.getByTestId(`recipe-card-${recipe.id}-meta`)
    ).not.toContainText(/incomplete/)
  } finally {
    await cleanupFood(recipe.id)
    await cleanupFood(legacy.id)
    await cleanupFood(rice.id)
  }
})
