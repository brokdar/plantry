import type { Page } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  expect,
  seedComposedFood,
  seedLeafFood,
  test,
  uid,
} from "./helpers"

async function searchFor(page: Page, text: string) {
  const res = page.waitForResponse(
    (r) =>
      r.url().includes("/api/foods") &&
      r
        .url()
        .includes(`search=${encodeURIComponent(text).replace(/%20/g, "+")}`)
  )
  await page.getByTestId("ingredients-search").fill(text)
  await res
}

test.describe("Ingredients table", () => {
  test("rows show name, kcal/100 g, section, used in and source", async ({
    page,
  }) => {
    const tag = uid()
    const chicken = await seedLeafFood({
      name: `Table Chicken ${tag}`,
      kcal_100g: 165,
    })
    const recipe = await seedComposedFood({
      name: `Table Recipe ${tag}`,
      role: "main",
      children: [
        {
          child_id: chicken.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })
    try {
      await page.goto("/recipes/ingredients")
      await searchFor(page, tag)

      const row = page.getByTestId(`ingredient-row-${chicken.id}`)
      await expect(row.getByRole("rowheader")).toHaveText(chicken.name)
      await expect(row).toContainText("165")
      await expect(row).toContainText("1 recipe")
      await expect(row).toContainText("Manual")
      await expect(
        row.getByRole("combobox", { name: `Store section for ${chicken.name}` })
      ).toContainText("auto")

      await row.getByRole("link", { name: chicken.name }).click()
      await expect(page).toHaveURL(
        new RegExp(`/recipes/ingredients/${chicken.id}/edit$`)
      )
    } finally {
      await cleanupFood(recipe.id)
      await cleanupFood(chicken.id)
    }
  })

  test("F8-AC3: changing a section persists through PUT shop-section", async ({
    page,
  }) => {
    const tag = uid()
    const peas = await seedLeafFood({ name: `Table Peas ${tag}` })
    try {
      await page.goto("/recipes/ingredients")
      await searchFor(page, tag)
      const section = page.getByRole("combobox", {
        name: `Store section for ${peas.name}`,
      })

      const put = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/api/foods/${peas.id}/shop-section`) &&
          r.request().method() === "PUT"
      )
      await section.click()
      await page.getByRole("option", { name: "Frozen" }).click()
      expect((await put).ok()).toBeTruthy()
      await expect(section).toHaveText("Frozen")

      await page.reload()
      await expect(section).toHaveText("Frozen")

      const ctx = await apiRequest.newContext({ baseURL: API })
      const food = (await (await ctx.get(`/api/foods/${peas.id}`)).json()) as {
        shop_section: string
        shop_section_source: string
      }
      await ctx.dispose()
      expect(food).toMatchObject({
        shop_section: "frozen",
        shop_section_source: "user",
      })

      const revert = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/api/foods/${peas.id}/shop-section`) &&
          r.request().method() === "PUT"
      )
      await section.click()
      await page.getByRole("option", { name: "Automatic" }).click()
      await revert
      await expect(section).toContainText("auto")
    } finally {
      await cleanupFood(peas.id)
    }
  })

  test("a search without results says so; New ingredient opens the form", async ({
    page,
  }) => {
    await page.goto("/recipes/ingredients")
    await searchFor(page, `zzz-${uid()}`)
    await expect(page.getByTestId("ingredients-empty")).toContainText(
      "No ingredients match"
    )
    await page.getByRole("link", { name: "New ingredient" }).click()
    await expect(page).toHaveURL(/\/recipes\/ingredients\/new$/)
  })
})
