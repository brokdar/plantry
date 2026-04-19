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
      await page.goto(`/components/${comp.id}/edit`)
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

      // Editor redirects to list on save — the updated card is visible.
      await page.getByTestId("catalog-search").fill(tag)
      await page.waitForResponse((r) => r.url().includes(`search=${tag}`))
      await expect(page.getByTestId(`component-card-${comp.id}`)).toContainText(
        `Updated ${tag}`
      )
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
      await test.step("API returns per-portion nutrition", async () => {
        // 400g at 200kcal/100g = 800kcal total, /2 portions = 400 per portion.
        const ctx = await apiRequest.newContext({ baseURL: API })
        const res = await ctx.get(`/api/components/${comp.id}/nutrition`)
        expect(res.ok()).toBeTruthy()
        const nut = (await res.json()) as { kcal: number; protein: number }
        expect(nut.kcal).toBe(400)
        expect(nut.protein).toBe(40)
        await ctx.dispose()
      })

      await page.goto(`/components/${comp.id}/edit`)
      const panel = page.getByTestId("section-card-nutrition")
      const portionBtn = page.getByRole("button", { name: /per portion/i })
      const totalBtn = page.getByRole("button", { name: /^total$/i })

      await test.step("default view shows per-portion kcal", async () => {
        await expect(panel).toContainText("400")
        await expect(panel).toContainText("kcal")
        await expect(portionBtn).toHaveAttribute("aria-pressed", "true")
      })

      await test.step("Total toggle multiplies kcal by reference portions", async () => {
        await totalBtn.click()
        await expect(totalBtn).toHaveAttribute("aria-pressed", "true")
        await expect(panel).toContainText("800")
      })

      await test.step("Per portion toggle restores the per-portion value", async () => {
        await portionBtn.click()
        await expect(portionBtn).toHaveAttribute("aria-pressed", "true")
        await expect(panel).toContainText("400")
      })
    } finally {
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
    }
  })

  test("editor ingredient combobox searches, selects, and computes grams", async ({
    page,
  }) => {
    const tag = uid()
    const ing = await seedIngredient({
      name: `Combo Chicken ${tag}`,
      kcal_100g: 165,
      protein_100g: 31,
    })
    const comp = await seedComponent({
      name: `Combo Recipe ${tag}`,
      role: "main",
      reference_portions: 1,
    })

    try {
      await page.goto(`/components/${comp.id}/edit`)

      const trigger = page.getByTestId("ingredient-row-0-combobox")
      const search = page.getByTestId("ingredient-row-0-combobox-search")
      const option = page.getByTestId(
        `ingredient-row-0-combobox-option-${ing.id}`
      )
      const amountInput = page.locator('input[name="ingredients.0.amount"]')
      const gramsInput = page.getByTestId("ingredient-row-0-grams")

      await test.step("open the combobox from an empty ingredient row", async () => {
        await page.getByRole("button", { name: /add ingredient/i }).click()
        await trigger.click()
        await expect(search).toBeFocused()
      })

      await test.step("typeahead surfaces the seeded ingredient with macro hint", async () => {
        const resp = page.waitForResponse(
          (r) => r.url().includes("/api/ingredients") && r.url().includes(tag)
        )
        await search.pressSequentially(`Combo Chicken ${tag}`, { delay: 30 })
        await resp

        await expect(option).toBeVisible()
        await expect(option).toContainText("165 kcal")
        await expect(option).toContainText("31P")
      })

      await test.step("selecting populates the row and closes the popover", async () => {
        await option.click()
        await expect(trigger).toContainText(`Combo Chicken ${tag}`)
        await expect(search).toHaveCount(0)
        await expect(amountInput).toHaveValue("100")
        await expect(gramsInput).toHaveValue("100.0")
      })

      await test.step("changing amount recomputes grams for matching unit", async () => {
        await amountInput.fill("250")
        await expect(gramsInput).toHaveValue("250.0")
      })

      await test.step("save persists the ingredient row to the backend", async () => {
        const resp = page.waitForResponse(
          (r) =>
            r.url().includes(`/api/components/${comp.id}`) &&
            r.request().method() === "PUT"
        )
        await page.getByRole("button", { name: /^save$/i }).click()
        const response = await resp
        expect(response.status()).toBe(200)
      })

      await test.step("reloading the editor shows the persisted ingredient row", async () => {
        await page.goto(`/components/${comp.id}/edit`)
        await expect(trigger).toContainText(`Combo Chicken ${tag}`)
        await expect(amountInput).toHaveValue("250")
      })
    } finally {
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
    }
  })

  test("editor add-instruction adds a numbered step that persists", async ({
    page,
  }) => {
    const tag = uid()
    const comp = await seedComponent({
      name: `Steps Recipe ${tag}`,
      role: "main",
      reference_portions: 1,
    })

    try {
      await page.goto(`/components/${comp.id}/edit`)

      await page.getByRole("button", { name: /add step/i }).click()

      const stepInput = page.locator('textarea[name="instructions.0.text"]')
      await stepInput.fill("Mix everything together")

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/components/${comp.id}`) &&
          r.request().method() === "PUT"
      )
      await page.getByRole("button", { name: /^save$/i }).click()
      const response = await resp
      expect(response.status()).toBe(200)

      await page.goto(`/components/${comp.id}/edit`)
      const persistedStep = page.locator('textarea[name="instructions.0.text"]')
      await expect(persistedStep).toHaveValue("Mix everything together")
    } finally {
      await cleanupComponent(comp.id)
    }
  })

  test("editor tag add round-trips to persisted editor", async ({ page }) => {
    const tag = uid()
    const newTag = `quick-${tag}`
    const comp = await seedComponent({
      name: `Tag Recipe ${tag}`,
      role: "main",
      reference_portions: 1,
    })

    try {
      await page.goto(`/components/${comp.id}/edit`)

      const tagInput = page.getByPlaceholder(/spicy, quick|scharf, schnell/)
      await tagInput.fill(newTag)
      await tagInput.press("Enter")
      // Inline tag pill appears immediately, before save.
      await expect(page.getByText(newTag, { exact: true })).toBeVisible()

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/components/${comp.id}`) &&
          r.request().method() === "PUT"
      )
      await page.getByRole("button", { name: /^save$/i }).click()
      const response = await resp
      expect(response.status()).toBe(200)

      await page.goto(`/components/${comp.id}/edit`)
      await expect(page.getByText(newTag, { exact: true })).toBeVisible()
    } finally {
      await cleanupComponent(comp.id)
    }
  })

  test("editor sticky action bar Cancel returns to /components without saving", async ({
    page,
  }) => {
    const tag = uid()
    const comp = await seedComponent({
      name: `Cancel Recipe ${tag}`,
      role: "main",
      reference_portions: 1,
    })

    try {
      await page.goto(`/components/${comp.id}/edit`)

      // Make a dirty change that should NOT persist.
      const nameInput = page.getByLabel(/^name/i)
      await nameInput.fill(`Edited ${tag}`)

      await page.getByRole("link", { name: /^cancel$/i }).click()
      await expect(page).toHaveURL(/\/components\/?$/)

      // Reload editor — name unchanged because we navigated away without saving.
      await page.goto(`/components/${comp.id}/edit`)
      await expect(page.getByLabel(/^name/i)).toHaveValue(
        `Cancel Recipe ${tag}`
      )
    } finally {
      await cleanupComponent(comp.id)
    }
  })
})
