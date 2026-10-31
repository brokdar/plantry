import type { Page } from "@playwright/test"

import {
  cleanupFood,
  expect,
  seedComposedFood,
  seedLeafFood,
  test,
} from "./helpers"

/** A digits-only tag: a hex uid could spell a diet keyword ("beef"). */
function digitTag(): string {
  return String(Math.floor(Math.random() * 1e9)).padStart(9, "0")
}

function child(id: number, sort_order = 0) {
  return { child_id: id, amount: 100, unit: "g", grams: 100, sort_order }
}

async function searchFoods(page: Page, testId: string, tag: string) {
  const res = page.waitForResponse(
    (r) => r.url().includes("/api/foods") && r.url().includes(`search=${tag}`)
  )
  await page.getByTestId(testId).fill(tag)
  await res
}

test.describe("Diet class (AC-7)", () => {
  test("the Ingredients table shows each class and saves an override", async ({
    page,
  }) => {
    const tag = digitTag()
    const [chicken, salmon, feta, chickpeas] = await Promise.all(
      ["Hähnchenbrust", "Lachs", "Feta", "Kichererbsen"].map((name) =>
        seedLeafFood({ name: `${name} ${tag}` })
      )
    )
    try {
      await page.goto("/recipes/ingredients")
      await searchFoods(page, "ingredients-search", tag)

      const diet = (id: number) => page.getByTestId(`ingredient-diet-${id}`)
      await expect(diet(chicken.id)).toHaveText("With meatauto")
      await expect(diet(salmon.id)).toHaveText("Pescetarianauto")
      await expect(diet(feta.id)).toHaveText("Vegetarianauto")
      await expect(diet(chickpeas.id)).toHaveText("Veganauto")
      await expect(
        page.getByRole("combobox", { name: `Diet for Feta ${tag}` })
      ).toBeVisible()

      const saved = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/foods/${feta.id}/diet-class`) &&
          r.request().method() === "PUT"
      )
      await diet(feta.id).click()
      await page.getByRole("option", { name: "Vegan" }).click()
      expect((await (await saved).json()).diet_class_source).toBe("user")
      await expect(diet(feta.id)).toHaveText("Vegan")

      const reverted = page.waitForResponse((r) =>
        r.url().includes(`/api/foods/${feta.id}/diet-class`)
      )
      await diet(feta.id).click()
      await page.getByRole("option", { name: "Automatic" }).click()
      await reverted
      await expect(diet(feta.id)).toHaveText("Vegetarianauto")
    } finally {
      await Promise.all(
        [chicken, salmon, feta, chickpeas].map((f) => cleanupFood(f.id))
      )
    }
  })

  test("the library's Diet chip filters and cards show the diet icon", async ({
    page,
  }) => {
    const tag = digitTag()
    const chickpeas = await seedLeafFood({ name: `Kichererbsen ${tag}` })
    const chicken = await seedLeafFood({ name: `Hähnchenbrust ${tag}` })
    const vegan = await seedComposedFood({
      name: `Hummus ${tag}`,
      role: "main",
      children: [child(chickpeas.id)],
    })
    const meat = await seedComposedFood({
      name: `Chicken bowl ${tag}`,
      role: "main",
      children: [child(chicken.id), child(chickpeas.id, 1)],
    })
    try {
      await page.goto("/recipes")
      await searchFoods(page, "recipes-search", tag)

      await expect(
        page
          .getByTestId(`recipe-card-${vegan.id}-meta`)
          .getByRole("img", { name: "Diet: Vegan" })
      ).toBeVisible()
      await expect(
        page
          .getByTestId(`recipe-card-${meat.id}-meta`)
          .getByRole("img", { name: "Diet: With meat" })
      ).toBeVisible()

      const filtered = page.waitForResponse(
        (r) =>
          r.url().includes("/api/foods") && r.url().includes("diet=vegetarian")
      )
      await page.getByTestId("recipes-filter-diet").click()
      await page.getByRole("option", { name: "Vegetarian" }).click()
      await filtered
      await expect(page).toHaveURL(/diet=vegetarian/)
      await expect(page.getByTestId(`recipe-card-${vegan.id}`)).toBeVisible()
      await expect(page.getByTestId(`recipe-card-${meat.id}`)).toHaveCount(0)
      await expect(page.getByTestId("recipes-filter-diet")).toHaveText(
        "Vegetarian"
      )
    } finally {
      await cleanupFood(vegan.id)
      await cleanupFood(meat.id)
      await cleanupFood(chickpeas.id)
      await cleanupFood(chicken.id)
    }
  })

  test("recipe detail shows the derived class, its override, and warns when unknown", async ({
    page,
  }) => {
    const tag = digitTag()
    const chickpeas = await seedLeafFood({ name: `Kichererbsen ${tag}` })
    const feta = await seedLeafFood({ name: `Feta ${tag}` })
    const mystery = await seedLeafFood({ name: `Qwrtzpulver ${tag}` })
    const salad = await seedComposedFood({
      name: `Feta salad ${tag}`,
      role: "main",
      children: [child(chickpeas.id), child(feta.id, 1)],
    })
    const unknown = await seedComposedFood({
      name: `Mystery stew ${tag}`,
      role: "main",
      children: [child(chickpeas.id), child(mystery.id, 1)],
    })
    try {
      await page.goto(`/recipes/${salad.id}`)
      const diet = page.getByTestId("recipe-diet")
      await expect(diet).toHaveText("Vegetarianauto")
      await expect(page.getByTestId("diet-unknown-warning")).toHaveCount(0)

      const saved = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/foods/${salad.id}/diet-class`) &&
          r.request().method() === "PUT"
      )
      await diet.click()
      await page.getByRole("option", { name: "Vegan" }).click()
      await saved
      await expect(diet).toHaveText("Vegan")

      await page.goto(`/recipes/${unknown.id}`)
      await expect(page.getByTestId("recipe-diet")).toHaveText("Unknownauto")
      const warning = page.getByTestId("diet-unknown-warning")
      await expect(warning).toContainText("Diet unknown")
      await expect(
        warning.getByRole("link", { name: `Qwrtzpulver ${tag}` })
      ).toHaveAttribute("href", `/recipes/ingredients/${mystery.id}/edit`)
    } finally {
      await cleanupFood(salad.id)
      await cleanupFood(unknown.id)
      await Promise.all(
        [chickpeas, feta, mystery].map((f) => cleanupFood(f.id))
      )
    }
  })
})
