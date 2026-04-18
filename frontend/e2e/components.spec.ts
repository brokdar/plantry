import { test, expect, apiRequest } from "./helpers"

import {
  API,
  cleanupComponent,
  cleanupIngredient,
  seedComponent,
  seedIngredient,
  uid,
} from "./helpers"

test.describe("Component Library", () => {
  test("create a component via the form", async ({ page }) => {
    const tag = uid()
    const ing = await seedIngredient({
      name: `E2E Chicken ${tag}`,
      kcal_100g: 165,
      protein_100g: 31,
    })
    let createdId: number | undefined

    try {
      await page.goto("/components/new")
      await page.getByLabel(/^name/i).fill(`Curry ${tag}`)
      await page.getByLabel(/servings/i).fill("2")

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/components") &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(201)

      const body = (await response.json()) as { id: number }
      createdId = body.id

      // List view may be paginated; narrow by the unique tag to locate the card.
      await page.getByTestId("catalog-search").fill(tag)
      await page.waitForResponse(
        (res) =>
          res.url().includes("/api/components") &&
          res.url().includes(`search=${tag}`)
      )
      await expect(
        page.getByTestId(`component-card-${createdId}`)
      ).toBeVisible()
    } finally {
      if (createdId) await cleanupComponent(createdId)
      await cleanupIngredient(ing.id)
    }
  })

  test("search filters components by name", async ({ page }) => {
    const tag = uid()
    const c1 = await seedComponent({
      name: `Chicken Curry ${tag}`,
      role: "main",
    })
    const c2 = await seedComponent({
      name: `Tofu Bowl ${tag}`,
      role: "standalone",
    })
    const c3 = await seedComponent({
      name: `Pasta Sauce ${tag}`,
      role: "sauce",
    })

    try {
      await page.goto("/components")

      // Narrow to seeded items only — shared DB accumulates over test runs.
      await page.getByTestId("catalog-search").fill(tag)
      await page.waitForResponse(
        (res) =>
          res.url().includes("/api/components") &&
          res.url().includes(`search=${tag}`)
      )

      await expect(page.getByTestId(`component-card-${c1.id}`)).toBeVisible()
      await expect(page.getByTestId(`component-card-${c2.id}`)).toBeVisible()
      await expect(page.getByTestId(`component-card-${c3.id}`)).toBeVisible()

      await page.getByTestId("catalog-search").fill(`chicken ${tag}`)
      await page.waitForResponse((res) => res.url().includes("/api/components"))

      await expect(page.getByTestId(`component-card-${c1.id}`)).toBeVisible()
      await expect(page.getByTestId(`component-card-${c2.id}`)).toHaveCount(0)
      await expect(page.getByTestId(`component-card-${c3.id}`)).toHaveCount(0)
    } finally {
      await cleanupComponent(c1.id)
      await cleanupComponent(c2.id)
      await cleanupComponent(c3.id)
    }
  })

  test("edit a component", async ({ page }) => {
    const tag = uid()
    const comp = await seedComponent({ name: `Edit Test ${tag}`, role: "main" })

    try {
      await page.goto(`/components/${comp.id}`)
      await expect(page.getByText(`Edit Test ${tag}`)).toBeVisible()

      await page.getByRole("link", { name: /edit/i }).click()
      await expect(page.getByLabel(/^name/i)).toHaveValue(`Edit Test ${tag}`)

      await page.getByLabel(/^name/i).fill(`Updated ${tag}`)

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/components/${comp.id}`) &&
          res.request().method() === "PUT"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(200)

      await expect(page.getByText(`Updated ${tag}`)).toBeVisible()
    } finally {
      await cleanupComponent(comp.id)
    }
  })

  test("delete a component", async ({ page }) => {
    const tag = uid()
    const keep = await seedComponent({ name: `Keep ${tag}`, role: "main" })
    const toDelete = await seedComponent({
      name: `Delete ${tag}`,
      role: "side_veg",
    })

    try {
      await page.goto("/components")
      await page.getByTestId("catalog-search").fill(tag)
      await page.waitForResponse(
        (res) =>
          res.url().includes("/api/components") &&
          res.url().includes(`search=${tag}`)
      )
      await expect(
        page.getByTestId(`component-card-${toDelete.id}`)
      ).toBeVisible()

      await page.getByTestId(`component-card-${toDelete.id}-menu`).click()
      await page.getByTestId(`component-card-${toDelete.id}-delete`).click()

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/components/${toDelete.id}`) &&
          res.request().method() === "DELETE"
      )
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click()
      await responsePromise

      await expect(
        page.getByTestId(`component-card-${toDelete.id}`)
      ).toHaveCount(0)
      await expect(page.getByTestId(`component-card-${keep.id}`)).toBeVisible()
    } finally {
      await cleanupComponent(keep.id)
    }
  })

  test("nutrition endpoint returns correct values", async ({ page }) => {
    const tag = uid()
    const ing = await seedIngredient({
      name: `Nut Chicken ${tag}`,
      kcal_100g: 200,
      protein_100g: 20,
      fat_100g: 10,
      carbs_100g: 0,
    })

    const comp = await seedComponent({
      name: `Nut Comp ${tag}`,
      role: "main",
      reference_portions: 2,
      ingredients: [
        {
          ingredient_id: ing.id,
          amount: 400,
          unit: "g",
          grams: 400,
          sort_order: 0,
        },
      ],
    })

    try {
      // Verify via API: 400g at 200kcal/100g = 800kcal total, /2 portions = 400
      const ctx = await apiRequest.newContext({ baseURL: API })
      const res = await ctx.get(`/api/components/${comp.id}/nutrition`)
      expect(res.ok()).toBeTruthy()
      const nut = (await res.json()) as { kcal: number; protein: number }
      expect(nut.kcal).toBe(400)
      expect(nut.protein).toBe(40)
      await ctx.dispose()

      // View detail page — nutrition should render
      await page.goto(`/components/${comp.id}`)
      await expect(page.getByText("400.0")).toBeVisible()
    } finally {
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
    }
  })
})
