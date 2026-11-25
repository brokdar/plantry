import {
  cleanupFood,
  expect,
  seedComposedFood,
  seedLeafFood,
  test,
} from "./helpers"

test.describe("Recipe attributes (Chef A1, ATTR-5)", () => {
  test("derived chips show; an edited chip is kept after a reload", async ({
    page,
  }) => {
    const uid = String(Math.floor(Math.random() * 1e9))
    const tomatoes = await seedLeafFood({
      name: `Tomaten ${uid}`,
      kcal_100g: 18,
      protein_100g: 0.9,
      fat_100g: 0.2,
      carbs_100g: 3.9,
    })
    const recipe = await seedComposedFood({
      name: `Tomatensuppe ${uid}`,
      role: "main",
      tags: ["Italienisch"],
      children: [
        {
          child_id: tomatoes.id,
          amount: 400,
          unit: "g",
          grams: 400,
          sort_order: 0,
        },
      ],
    })
    try {
      await page.goto(`/recipes/${recipe.id}`)
      const cuisine = page.getByTestId("attr-chip-cuisine")
      await expect(cuisine).toHaveText("CuisineItalian")
      await expect(page.getByTestId("attr-chip-heroes")).toContainText(
        `Tomaten ${uid}`
      )

      const saved = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/foods/${recipe.id}/attributes`) &&
          r.request().method() === "PUT"
      )
      await cuisine.click()
      await page.getByTestId("attr-option-french").click()
      expect((await (await saved).json()).sources.cuisine).toBe("user")
      await expect(cuisine).toContainText("French")

      await page.reload()
      await expect(cuisine).toContainText("French")
      await expect(cuisine).toHaveAttribute("data-source", "user")

      // Undo through the food change log puts the previous value back.
      await cuisine.click()
      await page.getByTestId("attr-option-thai").click()
      await expect(cuisine).toContainText("Thai")
      await page.getByRole("button", { name: "Undo" }).click()
      await expect(cuisine).toContainText("French")
    } finally {
      await cleanupFood(recipe.id)
      await cleanupFood(tomatoes.id)
    }
  })
})
