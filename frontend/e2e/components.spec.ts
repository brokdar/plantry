import { test, expect, apiRequest } from "./helpers"

import {
  API,
  cleanupFood,
  seedComposedFood,
  seedComposedWithStub,
  seedLeafFood,
  uid,
} from "./helpers"

test.describe("Component Library", () => {
  test("create a component via the form", async ({ page }) => {
    const tag = uid()
    const ing = await seedLeafFood({
      name: `E2E Chicken ${tag}`,
      kcal_100g: 165,
      protein_100g: 31,
    })
    let createdId: number | undefined

    try {
      await page.goto("/components/new")
      await page.getByLabel(/^name/i).fill(`Curry ${tag}`)
      await page.getByLabel(/servings/i).fill("2")

      // Composed foods require at least one child — pick the seeded leaf via
      // the inline combobox.
      await page.getByRole("button", { name: /add ingredient/i }).click()
      await page.getByTestId("ingredient-row-0-combobox").click()
      await page
        .getByTestId("ingredient-row-0-combobox-search")
        .fill(`E2E Chicken ${tag}`)
      await page
        .getByTestId(`ingredient-row-0-combobox-option-${ing.id}`)
        .click()

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") && res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(201)

      const body = (await response.json()) as { id: number }
      createdId = body.id

      // List view may be paginated; narrow by the unique tag to locate the card.
      const searchResp = page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") &&
          res.url().includes(`search=${tag}`)
      )
      await page.getByTestId("catalog-search").fill(tag)
      await searchResp
      await expect(
        page.getByTestId(`component-card-${createdId}`)
      ).toBeVisible()
    } finally {
      if (createdId) await cleanupFood(createdId)
      await cleanupFood(ing.id)
    }
  })

  test("search filters components by name", async ({ page }) => {
    const tag = uid()
    const { composed: c1, stub: s1 } = await seedComposedWithStub(
      { name: `Chicken Curry ${tag}`, role: "main" },
      tag
    )
    const { composed: c2, stub: s2 } = await seedComposedWithStub(
      { name: `Tofu Bowl ${tag}`, role: "standalone" },
      tag
    )
    const { composed: c3, stub: s3 } = await seedComposedWithStub(
      { name: `Pasta Sauce ${tag}`, role: "sauce" },
      tag
    )

    try {
      await page.goto("/components")

      // Narrow to seeded items only — shared DB accumulates over test runs.
      const resp1 = page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") &&
          res.url().includes(`search=${tag}`)
      )
      await page.getByTestId("catalog-search").fill(tag)
      await resp1

      await expect(page.getByTestId(`component-card-${c1.id}`)).toBeVisible()
      await expect(page.getByTestId(`component-card-${c2.id}`)).toBeVisible()
      await expect(page.getByTestId(`component-card-${c3.id}`)).toBeVisible()

      const resp2 = page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") &&
          res.url().includes("chicken") &&
          res.url().includes(tag)
      )
      await page.getByTestId("catalog-search").fill(`chicken ${tag}`)
      await resp2

      await expect(page.getByTestId(`component-card-${c1.id}`)).toBeVisible()
      await expect(page.getByTestId(`component-card-${c2.id}`)).toHaveCount(0)
      await expect(page.getByTestId(`component-card-${c3.id}`)).toHaveCount(0)
    } finally {
      await cleanupFood(c1.id)
      await cleanupFood(c2.id)
      await cleanupFood(c3.id)
      await cleanupFood(s1.id)
      await cleanupFood(s2.id)
      await cleanupFood(s3.id)
    }
  })

  test("edit a component", async ({ page }) => {
    const tag = uid()
    const { composed: comp, stub } = await seedComposedWithStub(
      { name: `Edit Test ${tag}`, role: "main" },
      tag
    )

    try {
      await page.goto(`/components/${comp.id}/edit`)
      await expect(page.getByLabel(/^name/i)).toHaveValue(`Edit Test ${tag}`)

      await page.getByLabel(/^name/i).fill(`Updated ${tag}`)

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/foods/${comp.id}`) &&
          res.request().method() === "PUT"
      )
      await page.getByRole("button", { name: /save/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(200)

      // Editor redirects to list on save — the updated card is visible.
      const editSearchResp = page.waitForResponse((r) =>
        r.url().includes(`search=${tag}`)
      )
      await page.getByTestId("catalog-search").fill(tag)
      await editSearchResp
      await expect(page.getByTestId(`component-card-${comp.id}`)).toContainText(
        `Updated ${tag}`
      )
    } finally {
      await cleanupFood(comp.id)
      await cleanupFood(stub.id)
    }
  })

  test("delete a component", async ({ page }) => {
    const tag = uid()
    const { composed: keep, stub: keepStub } = await seedComposedWithStub(
      { name: `Keep ${tag}`, role: "main" },
      tag
    )
    const { composed: toDelete, stub: delStub } = await seedComposedWithStub(
      { name: `Delete ${tag}`, role: "side_veg" },
      tag
    )

    try {
      await page.goto("/components")
      const deleteSearchResp = page.waitForResponse(
        (res) =>
          res.url().includes("/api/foods") &&
          res.url().includes(`search=${tag}`)
      )
      await page.getByTestId("catalog-search").fill(tag)
      await deleteSearchResp
      await expect(
        page.getByTestId(`component-card-${toDelete.id}`)
      ).toBeVisible()

      await page.getByTestId(`component-card-${toDelete.id}-menu`).click()
      await page.getByTestId(`component-card-${toDelete.id}-delete`).click()

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/foods/${toDelete.id}`) &&
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
      await cleanupFood(keep.id)
      await cleanupFood(keepStub.id)
      await cleanupFood(delStub.id)
    }
  })

  test("nutrition endpoint returns correct values", async ({ page }) => {
    const tag = uid()
    const ing = await seedLeafFood({
      name: `Nut Chicken ${tag}`,
      kcal_100g: 200,
      protein_100g: 20,
      fat_100g: 10,
      carbs_100g: 0,
    })

    const comp = await seedComposedFood({
      name: `Nut Comp ${tag}`,
      role: "main",
      reference_portions: 2,
      children: [
        {
          child_id: ing.id,
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
        const res = await ctx.get(`/api/foods/${comp.id}/nutrition`)
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
      await cleanupFood(comp.id)
      await cleanupFood(ing.id)
    }
  })

  test("editor ingredient combobox searches, selects, and computes grams", async ({
    page,
  }) => {
    const tag = uid()
    const ing = await seedLeafFood({
      name: `Combo Chicken ${tag}`,
      kcal_100g: 165,
      protein_100g: 31,
    })
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const comp = await seedComposedFood({
      name: `Combo Recipe ${tag}`,
      role: "main",
      reference_portions: 1,
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await page.goto(`/components/${comp.id}/edit`)

      // The seed populates row 0 with a stub child; the test exercises the
      // appended row 1.
      const trigger = page.getByTestId("ingredient-row-1-combobox")
      const search = page.getByTestId("ingredient-row-1-combobox-search")
      const option = page.getByTestId(
        `ingredient-row-1-combobox-option-${ing.id}`
      )
      const amountInput = page.locator('input[name="children.1.amount"]')
      const gramsInput = page.getByTestId("ingredient-row-1-grams")

      await test.step("open the combobox from an empty ingredient row", async () => {
        await page.getByRole("button", { name: /add ingredient/i }).click()
        await trigger.click()
        await expect(search).toBeFocused()
      })

      await test.step("typeahead surfaces the seeded ingredient with macro hint", async () => {
        const resp = page.waitForResponse(
          (r) => r.url().includes("/api/foods") && r.url().includes(tag)
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
        await expect(gramsInput).toHaveValue("100")
      })

      await test.step("changing amount recomputes grams for matching unit", async () => {
        await amountInput.fill("250")
        await expect(gramsInput).toHaveValue("250")
      })

      await test.step("save persists the ingredient row to the backend", async () => {
        const resp = page.waitForResponse(
          (r) =>
            r.url().includes(`/api/foods/${comp.id}`) &&
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
      await cleanupFood(comp.id)
      await cleanupFood(ing.id)
      await cleanupFood(stub.id)
    }
  })

  test("editor add-instruction adds a numbered step that persists", async ({
    page,
  }) => {
    const tag = uid()
    const { composed: comp, stub } = await seedComposedWithStub(
      { name: `Steps Recipe ${tag}`, role: "main", reference_portions: 1 },
      tag
    )

    try {
      await page.goto(`/components/${comp.id}/edit`)

      await page.getByRole("button", { name: /add step/i }).click()

      const stepInput = page.locator('textarea[name="instructions.0.text"]')
      await stepInput.fill("Mix everything together")

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/foods/${comp.id}`) &&
          r.request().method() === "PUT"
      )
      await page.getByRole("button", { name: /^save$/i }).click()
      const response = await resp
      expect(response.status()).toBe(200)

      await page.goto(`/components/${comp.id}/edit`)
      const persistedStep = page.locator('textarea[name="instructions.0.text"]')
      await expect(persistedStep).toHaveValue("Mix everything together")
    } finally {
      await cleanupFood(comp.id)
      await cleanupFood(stub.id)
    }
  })

  test("editor tag add round-trips to persisted editor", async ({ page }) => {
    const tag = uid()
    const newTag = `quick-${tag}`
    const { composed: comp, stub } = await seedComposedWithStub(
      { name: `Tag Recipe ${tag}`, role: "main", reference_portions: 1 },
      tag
    )

    try {
      await page.goto(`/components/${comp.id}/edit`)

      const tagInput = page.getByPlaceholder(/spicy, quick|scharf, schnell/)
      await tagInput.fill(newTag)
      await tagInput.press("Enter")
      // Inline tag pill appears immediately, before save.
      await expect(page.getByText(newTag, { exact: true })).toBeVisible()

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/foods/${comp.id}`) &&
          r.request().method() === "PUT"
      )
      await page.getByRole("button", { name: /^save$/i }).click()
      const response = await resp
      expect(response.status()).toBe(200)

      await page.goto(`/components/${comp.id}/edit`)
      await expect(page.getByText(newTag, { exact: true })).toBeVisible()
    } finally {
      await cleanupFood(comp.id)
      await cleanupFood(stub.id)
    }
  })

  test("editor sticky action bar Cancel returns to /components without saving", async ({
    page,
  }) => {
    const tag = uid()
    const { composed: comp, stub } = await seedComposedWithStub(
      { name: `Cancel Recipe ${tag}`, role: "main", reference_portions: 1 },
      tag
    )

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
      await cleanupFood(comp.id)
      await cleanupFood(stub.id)
    }
  })
})
