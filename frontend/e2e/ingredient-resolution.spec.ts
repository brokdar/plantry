import { test, expect } from "@playwright/test"

const API_BASE = "http://localhost:8080"

async function createIngredientAPI(name: string): Promise<number> {
  const res = await fetch(`${API_BASE}/api/ingredients`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name,
      kcal_100g: 100,
      protein_100g: 10,
      fat_100g: 5,
      carbs_100g: 15,
    }),
  })
  const data = await res.json()
  return data.id
}

async function deleteIngredientAPI(id: number): Promise<void> {
  await fetch(`${API_BASE}/api/ingredients/${id}`, { method: "DELETE" })
}

test.describe("Ingredient Resolution", () => {
  test("create ingredient from lookup", async ({ page }) => {
    const uid = crypto.randomUUID().slice(0, 8)
    const candidateName = `Chicken Breast ${uid}`

    await page.route("**/api/ingredients/lookup*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          results: [
            {
              name: candidateName,
              source: "fdc",
              barcode: null,
              fdc_id: 171077,
              image_url: null,
              existing_id: null,
              kcal_100g: 120,
              protein_100g: 22.5,
              fat_100g: 2.6,
              carbs_100g: 0,
              fiber_100g: 0,
              sodium_100g: 0.074,
              portions: [],
            },
          ],
          recommended_index: 0,
        }),
      })
    })

    let createdId: number | undefined
    try {
      await page.goto("/ingredients/new")

      // Search tab is visible by default
      const searchInput = page.getByPlaceholder(/search by name or barcode/i)
      await expect(searchInput).toBeVisible()
      await searchInput.fill("chicken breast")

      // Wait for mocked candidate to appear and click it
      await expect(page.getByText(candidateName)).toBeVisible()
      await page.getByText(candidateName).click()

      // Form should be populated with candidate data
      await expect(page.getByLabel(/name/i)).toHaveValue(candidateName)

      // Save and capture response
      const savePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/ingredients") &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await savePromise
      const body = await response.json()
      createdId = body.id

      await expect(page).toHaveURL(/\/ingredients$/)
    } finally {
      if (createdId) await deleteIngredientAPI(createdId)
    }
  })

  test("switch to manual tab and create ingredient", async ({ page }) => {
    const uid = crypto.randomUUID().slice(0, 8)
    const name = `Manual ingredient ${uid}`

    let createdId: number | undefined
    try {
      await page.goto("/ingredients/new")

      await page.getByRole("tab", { name: /manual/i }).click()

      await page.getByLabel(/name/i).fill(name)
      await page.getByLabel(/calories/i).fill("100")

      const savePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/ingredients") &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await savePromise
      const body = await response.json()
      createdId = body.id

      await expect(page).toHaveURL(/\/ingredients$/)
    } finally {
      if (createdId) await deleteIngredientAPI(createdId)
    }
  })

  test("barcode lookup flow", async ({ page }) => {
    const uid = crypto.randomUUID().slice(0, 8)
    const candidateName = `Barcode Product ${uid}`

    await page.route("**/api/ingredients/lookup*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          results: [
            {
              name: candidateName,
              source: "off",
              barcode: "0123456789012",
              fdc_id: null,
              image_url: null,
              existing_id: null,
              kcal_100g: 250,
              protein_100g: 8,
              fat_100g: 12,
              carbs_100g: 30,
              fiber_100g: 2,
              sodium_100g: 0.5,
              portions: [],
            },
          ],
          recommended_index: 0,
        }),
      })
    })

    let createdId: number | undefined
    try {
      await page.goto("/ingredients/new")

      // Open barcode scanner dialog
      await page.getByRole("button", { name: /scan barcode/i }).click()

      // Enter barcode in dialog and submit
      await page.getByPlaceholder("0123456789012").fill("0123456789012")
      await page
        .getByRole("dialog")
        .getByRole("button", { name: /scan barcode/i })
        .click()

      // Wait for mocked candidate to appear and click it
      await expect(page.getByText(candidateName)).toBeVisible()
      await page.getByText(candidateName).click()

      // Form should be populated
      await expect(page.getByLabel(/name/i)).toHaveValue(candidateName)

      // Save and capture response
      const savePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/ingredients") &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await savePromise
      const body = await response.json()
      createdId = body.id

      await expect(page).toHaveURL(/\/ingredients$/)
    } finally {
      if (createdId) await deleteIngredientAPI(createdId)
    }
  })

  test("edit mode: add and delete portion", async ({ page }) => {
    const uid = crypto.randomUUID().slice(0, 8)
    const ingredientName = `Portion test ${uid}`
    const ingredientId = await createIngredientAPI(ingredientName)

    try {
      await page.goto(`/ingredients/${ingredientId}/edit`)

      // Wait for the portions section to load
      await expect(
        page.getByRole("heading", { name: /portions/i })
      ).toBeVisible()

      // Fill in a new portion
      await page.getByPlaceholder(/e\.g\. cup/i).fill("cup")
      // Target the grams input by absence of name attribute (PortionsEditor uses
      // controlled state, so no name; macro fields get name via react-hook-form spread)
      await page.locator('input[type="number"]:not([name])').fill("240")

      // Add the portion
      const addPromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/ingredients/${ingredientId}/portions`) &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /add portion/i }).click()
      await addPromise

      // Verify portion appears
      await expect(page.getByText("cup")).toBeVisible()
      await expect(page.getByText("240g")).toBeVisible()

      // Delete the portion
      const deletePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/ingredients/${ingredientId}/portions`) &&
          res.request().method() === "DELETE"
      )
      await page.getByRole("button", { name: /delete cup/i }).click()
      await deletePromise

      // Verify portion is gone (server-filtered, use toHaveCount)
      await expect(page.getByText("cup")).toHaveCount(0)
    } finally {
      await deleteIngredientAPI(ingredientId)
    }
  })

  test("duplicate name shows error", async ({ page }) => {
    const uid = crypto.randomUUID().slice(0, 8)
    const name = `Duplicate test ${uid}`
    const seededId = await createIngredientAPI(name)

    try {
      await page.goto("/ingredients/new")

      await page.getByRole("tab", { name: /manual/i }).click()

      await page.getByLabel(/name/i).fill(name)
      await page.getByLabel(/calories/i).fill("200")

      const savePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/ingredients") &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      await savePromise

      // Error message about duplicate name should appear
      await expect(page.getByText(/already exists/i)).toBeVisible()
    } finally {
      await deleteIngredientAPI(seededId)
    }
  })
})
