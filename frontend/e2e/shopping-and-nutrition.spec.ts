import { request as apiRequest, expect, test } from "@playwright/test"

const API = "http://localhost:8080"

function uid() {
  return crypto.randomUUID().slice(0, 8)
}

async function seedSlot(name_key: string, icon: string, sort_order: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/settings/slots", {
    data: { name_key, icon, sort_order, active: true },
  })
  const body = (await res.json()) as { id: number; name_key: string }
  expect(res.ok(), `seed slot failed: ${JSON.stringify(body)}`).toBeTruthy()
  await ctx.dispose()
  return body
}

async function seedIngredient(
  name: string,
  kcal: number,
  protein: number,
  fat: number,
  carbs: number
) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/ingredients", {
    data: {
      name,
      source: "manual",
      kcal_100g: kcal,
      protein_100g: protein,
      fat_100g: fat,
      carbs_100g: carbs,
    },
  })
  const body = (await res.json()) as { id: number; name: string }
  expect(
    res.ok(),
    `seed ingredient failed: ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body
}

async function seedComponent(
  name: string,
  role: string,
  ingredientId: number,
  grams: number
) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/components", {
    data: {
      name,
      role,
      reference_portions: 1,
      ingredients: [{ ingredient_id: ingredientId, amount: grams, unit: "g" }],
    },
  })
  const body = (await res.json()) as { id: number; name: string }
  expect(
    res.ok(),
    `seed component failed: ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body
}

async function deletePlatesUsingSlot(slotId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const wRes = await ctx.get("/api/weeks?limit=100")
  const weeks = ((await wRes.json()) as { items: { id: number }[] }).items
  for (const w of weeks) {
    const det = await ctx.get(`/api/weeks/${w.id}`)
    const detail = (await det.json()) as {
      plates: { id: number; slot_id: number }[]
    }
    for (const p of detail.plates) {
      if (p.slot_id === slotId) {
        await ctx.delete(`/api/plates/${p.id}`)
      }
    }
  }
  await ctx.dispose()
}

async function cleanupSlot(id: number) {
  await deletePlatesUsingSlot(id)
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/settings/slots/${id}`)
  await ctx.dispose()
}

async function cleanupComponent(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/components/${id}`)
  await ctx.dispose()
}

async function cleanupIngredient(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/ingredients/${id}`)
  await ctx.dispose()
}

test.describe("Shopping List and Nutrition", () => {
  test("open shopping list and see aggregated grams", async ({ page }) => {
    const tag = uid()

    // Seed: ingredient at 100 kcal/100g, component with 300g of it, 1 ref portion
    const ing = await seedIngredient(
      `Chicken ${tag}`,
      100, // kcal/100g
      20, // protein
      5, // fat
      0 // carbs
    )
    const comp = await seedComponent(
      `Curry ${tag}`,
      "main",
      ing.id,
      300 // grams
    )
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 999)

    try {
      await page.goto("/")

      // Add a plate via the planner grid.
      const cell = page.getByTestId(`cell-0-${slot.id}`)
      await expect(cell).toBeVisible()

      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /add a meal/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Curry ${tag}`) })
        .click()
      await createPlateResp

      // Open shopping list.
      await page.getByRole("button", { name: /shopping/i }).click()

      const dialog = page.getByRole("dialog")

      // Wait for the sheet to open and data to load.
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // Ingredient should appear with 300g.
      await expect(dialog.getByText(new RegExp(`Chicken ${tag}`))).toBeVisible()
      await expect(dialog.getByText("300 g")).toBeVisible()

      // Check off the item.
      const checkbox = dialog.getByRole("checkbox")
      await checkbox.click()
      await expect(checkbox).toBeChecked()

      // Close and re-open — checked state should persist.
      await page.keyboard.press("Escape")
      await page.getByRole("button", { name: /shopping/i }).click()
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()
      await expect(dialog.getByRole("checkbox")).toBeChecked()
    } finally {
      await cleanupSlot(slot.id)
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
    }
  })

  test("open nutrition panel and see day bars", async ({ page }) => {
    const tag = uid()

    // 200g of ingredient at 200 kcal/100g → 400 kcal total, 1 ref portion
    const ing = await seedIngredient(`Rice ${tag}`, 200, 5, 1, 45)
    const comp = await seedComponent(`Bowl ${tag}`, "side_starch", ing.id, 200)
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 998)

    try {
      await page.goto("/")

      const cell = page.getByTestId(`cell-0-${slot.id}`)
      await expect(cell).toBeVisible()

      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /add a meal/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Bowl ${tag}`) })
        .click()
      await createPlateResp

      // Open nutrition panel.
      await page.getByRole("button", { name: /nutrition/i }).click()

      const panel = page.getByRole("dialog")
      await expect(
        panel.getByRole("heading", { name: /week nutrition/i })
      ).toBeVisible()

      // Day bar for Monday (day 0) should show kcal.
      await expect(panel.getByText("Mon")).toBeVisible()
      await expect(panel.getByText("400 kcal").first()).toBeVisible()

      // Week total row.
      await expect(panel.getByText("Week total")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
    }
  })
})
