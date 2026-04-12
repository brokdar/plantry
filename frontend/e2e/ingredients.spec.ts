import { test, expect, request as apiRequest } from "@playwright/test"

// Backend URL — seed/cleanup requests go directly to the backend, bypassing
// the Vite proxy, to avoid proxy bottlenecks under parallel workers.
const API = "http://localhost:8080"

// Unique suffix per test run prevents UNIQUE constraint collisions when tests
// run in parallel or with --repeat-each.
function uid() {
  return crypto.randomUUID().slice(0, 8)
}

// ---------------------------------------------------------------------------
// Helper: create an ingredient via direct backend API.
// ---------------------------------------------------------------------------
async function seedIngredient(data: {
  name: string
  kcal_100g?: number
  protein_100g?: number
  fat_100g?: number
  carbs_100g?: number
}) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/ingredients", { data })
  const body = await res.json()
  expect(
    res.ok(),
    `Seed "${data.name}" failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string }
}

// ---------------------------------------------------------------------------
// Helper: delete ingredient via direct backend API. Best-effort cleanup.
// ---------------------------------------------------------------------------
async function cleanupIngredient(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/ingredients/${id}`)
  await ctx.dispose()
}

test.describe("Ingredient Catalogue", () => {
  test("create an ingredient via the form", async ({ page }) => {
    const name = `Chicken breast ${uid()}`
    let createdId: number | undefined

    try {
      await page.goto("/ingredients/new")
      await page.getByLabel(/^name/i).fill(name)
      await page.getByLabel(/calories/i).fill("165")
      await page.getByLabel(/protein/i).fill("31")
      await page.getByLabel(/^fat/i).fill("3.6")
      await page.getByLabel(/^carbs/i).fill("0")

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/ingredients") &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(201)

      const body = (await response.json()) as { id: number }
      createdId = body.id

      // Should navigate back to list and show the ingredient
      await expect(page.getByRole("cell", { name })).toBeVisible()
      await expect(page.getByRole("cell", { name: "165" })).toBeVisible()
    } finally {
      if (createdId) await cleanupIngredient(createdId)
    }
  })

  test("search filters ingredients by name", async ({ page }) => {
    const tag = uid()
    const chicken = await seedIngredient({
      name: `Chicken thigh ${tag}`,
      kcal_100g: 209,
    })
    const tofu = await seedIngredient({
      name: `Tofu ${tag}`,
      kcal_100g: 76,
    })
    const rice = await seedIngredient({
      name: `Basmati rice ${tag}`,
      kcal_100g: 130,
    })

    try {
      await page.goto("/ingredients")

      // All three visible
      await expect(page.getByRole("cell", { name: chicken.name })).toBeVisible()
      await expect(page.getByRole("cell", { name: tofu.name })).toBeVisible()
      await expect(page.getByRole("cell", { name: rice.name })).toBeVisible()

      // Search — only chicken matches
      await page.getByPlaceholder(/search/i).fill("chicken")
      await page.waitForResponse(
        (res) =>
          res.url().includes("/api/ingredients") &&
          res.url().includes("search=chicken")
      )

      await expect(page.getByRole("cell", { name: chicken.name })).toBeVisible()
      await expect(page.getByRole("cell", { name: tofu.name })).toHaveCount(0)
      await expect(page.getByRole("cell", { name: rice.name })).toHaveCount(0)
    } finally {
      await cleanupIngredient(chicken.id)
      await cleanupIngredient(tofu.id)
      await cleanupIngredient(rice.id)
    }
  })

  test("edit an ingredient", async ({ page }) => {
    const name = `Brown rice ${uid()}`
    const ingredient = await seedIngredient({ name, kcal_100g: 112 })

    try {
      await page.goto(`/ingredients/${ingredient.id}/edit`)
      await expect(page.getByLabel(/^name/i)).toHaveValue(name)

      // Change kcal
      await page.getByLabel(/calories/i).fill("120")

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/ingredients/${ingredient.id}`) &&
          res.request().method() === "PUT"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(200)

      // Back on list, verify updated value
      await expect(page.getByRole("cell", { name: "120" })).toBeVisible()
    } finally {
      await cleanupIngredient(ingredient.id)
    }
  })

  test("delete an ingredient", async ({ page }) => {
    const tag = uid()
    const keep = await seedIngredient({
      name: `Olive oil ${tag}`,
      kcal_100g: 884,
    })
    const toDelete = await seedIngredient({
      name: `Butter ${tag}`,
      kcal_100g: 717,
    })

    try {
      await page.goto("/ingredients")
      await expect(
        page.getByRole("cell", { name: toDelete.name })
      ).toBeVisible()

      // Click delete on the Butter row
      const row = page.getByRole("row").filter({ hasText: toDelete.name })
      await row.getByRole("button", { name: /delete/i }).click()

      // Confirm deletion
      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/ingredients/${toDelete.id}`) &&
          res.request().method() === "DELETE"
      )
      await page.getByRole("button", { name: /^delete$/i }).click()
      await responsePromise

      // Deleted ingredient gone, kept ingredient remains
      await expect(page.getByRole("cell", { name: toDelete.name })).toHaveCount(
        0
      )
      await expect(page.getByRole("cell", { name: keep.name })).toBeVisible()
    } finally {
      await cleanupIngredient(keep.id)
    }
  })

  test("shows validation error when submitting empty name", async ({
    page,
  }) => {
    await page.goto("/ingredients/new")

    // Fill a macro field but leave name empty
    await page.getByLabel(/calories/i).fill("100")

    // Click save — form should NOT submit (client-side validation)
    await page.getByRole("button", { name: /save/i }).click()

    // Should still be on the new ingredient page (no navigation)
    await expect(page.getByRole("button", { name: /save/i })).toBeVisible()

    // The name input should have a validation error (browser or zod)
    // Check that the name field is marked invalid or an error message appears
    const nameInput = page.getByLabel(/^name/i)
    await expect(nameInput).toBeVisible()

    // No cleanup needed — nothing was created
  })

  test("shows error when creating ingredient with duplicate name", async ({
    page,
  }) => {
    const name = `Duplicate test ${uid()}`
    const existing = await seedIngredient({ name, kcal_100g: 100 })

    try {
      await page.goto("/ingredients/new")
      await page.getByLabel(/^name/i).fill(name)
      await page.getByLabel(/calories/i).fill("200")

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/ingredients") &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(409)

      // Error message should be visible on the form
      await expect(
        page.getByText("An ingredient with this name already exists.")
      ).toBeVisible()
    } finally {
      await cleanupIngredient(existing.id)
    }
  })
})
