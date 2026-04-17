import { expect, test } from "@playwright/test"

import {
  cleanupComponent,
  cleanupIngredient,
  cleanupSlot,
  seedComponent,
  seedIngredient,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Shopping List and Nutrition", () => {
  test("open shopping list and see aggregated grams", async ({ page }) => {
    const tag = uid()

    // Seed: ingredient at 100 kcal/100g, component with 300g of it, 1 ref portion
    const ing = await seedIngredient({
      name: `Chicken ${tag}`,
      kcal_100g: 100,
      protein_100g: 20,
      fat_100g: 5,
      carbs_100g: 0,
    })
    const comp = await seedComponent({
      name: `Curry ${tag}`,
      role: "main",
      ingredients: [
        {
          ingredient_id: ing.id,
          amount: 300,
          unit: "g",
          grams: 300,
          sort_order: 0,
        },
      ],
    })
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
      const checkbox = dialog.getByRole("checkbox", {
        name: new RegExp(`Chicken ${tag}`),
      })
      await checkbox.click()
      await expect(checkbox).toBeChecked()

      // Close and re-open — checked state should persist.
      await page.keyboard.press("Escape")
      await page.getByRole("button", { name: /shopping/i }).click()
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()
      await expect(
        dialog.getByRole("checkbox", { name: new RegExp(`Chicken ${tag}`) })
      ).toBeChecked()
    } finally {
      await cleanupSlot(slot.id)
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
    }
  })

  test("open nutrition panel and see day bars", async ({ page }) => {
    const tag = uid()

    // 200g of ingredient at 200 kcal/100g → 400 kcal total, 1 ref portion
    const ing = await seedIngredient({
      name: `Rice ${tag}`,
      kcal_100g: 200,
      protein_100g: 5,
      fat_100g: 1,
      carbs_100g: 45,
    })
    const comp = await seedComponent({
      name: `Bowl ${tag}`,
      role: "main",
      ingredients: [
        {
          ingredient_id: ing.id,
          amount: 200,
          unit: "g",
          grams: 200,
          sort_order: 0,
        },
      ],
    })
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
      await expect(panel.getByText(/\d+ kcal/).first()).toBeVisible()

      // Week total row.
      await expect(panel.getByText("Week total")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
    }
  })
})
