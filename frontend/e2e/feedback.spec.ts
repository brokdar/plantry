import { expect, apiRequest, test } from "./helpers"

import {
  API,
  cleanupComponent,
  cleanupIngredient,
  cleanupSlot,
  seedComponent,
  seedIngredient,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Plate feedback + AI memory loop", () => {
  test("mark a plate loved, then a new conversation's system prompt sees the tag", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.fb-${tag}`, "Moon", 999)
    const ing = await seedIngredient({ name: `Chicken ${tag}`, kcal_100g: 100 })
    const comp = await seedComponent({
      name: `Curry ${tag}`,
      role: "main",
      ingredients: [
        {
          ingredient_id: ing.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
      tags: [`spicy-${tag}`],
    })

    try {
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()

      // Seed a plate through the UI — reuse the existing planner flow.
      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /add a meal/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Curry ${tag}`) })
        .click()
      await createPlateResp
      await expect(cell.getByText(`Curry ${tag}`)).toBeVisible()

      // Click "Loved" on the feedback bar. Wait for the PUT feedback response.
      const feedbackResp = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+\/feedback$/.test(r.url()) &&
          r.request().method() === "PUT"
      )
      await cell.getByRole("button", { name: "Loved" }).click()
      await feedbackResp

      // The pressed state flips on.
      await expect(cell.getByRole("button", { name: "Loved" })).toHaveAttribute(
        "aria-pressed",
        "true"
      )

      // Fetch the debug endpoint directly (dev-mode only) and assert the
      // component's tag landed in the profile preferences section of the
      // system prompt. This is the full round-trip: feedback → profile →
      // ComposePrompt.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const weekResp = await ctx.get("/api/weeks/current")
      const week = (await weekResp.json()) as { id: number }
      const prompt = await ctx.get(
        `/api/ai/debug/system-prompt?week_id=${week.id}`
      )
      expect(prompt.ok()).toBeTruthy()
      const body = (await prompt.json()) as { system_prompt: string }
      expect(body.system_prompt).toContain(`spicy-${tag}`)
      await ctx.dispose()
    } finally {
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
      await cleanupSlot(slot.id)
    }
  })

  test("clicking the active status clears feedback", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.fb2-${tag}`, "Moon", 998)
    const ing = await seedIngredient({ name: `Rice ${tag}`, kcal_100g: 100 })
    const comp = await seedComponent({
      name: `Bowl ${tag}`,
      role: "main",
      ingredients: [
        {
          ingredient_id: ing.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)

      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /add a meal/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Bowl ${tag}`) })
        .click()
      await createPlateResp

      const put = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+\/feedback$/.test(r.url()) &&
          r.request().method() === "PUT"
      )
      await cell.getByRole("button", { name: "Cooked" }).click()
      await put
      await expect(
        cell.getByRole("button", { name: "Cooked" })
      ).toHaveAttribute("aria-pressed", "true")

      const del = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+\/feedback$/.test(r.url()) &&
          r.request().method() === "DELETE"
      )
      await cell.getByRole("button", { name: "Cooked" }).click()
      await del
      await expect(
        cell.getByRole("button", { name: "Cooked" })
      ).not.toHaveAttribute("aria-pressed", "true")
    } finally {
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
      await cleanupSlot(slot.id)
    }
  })
})
