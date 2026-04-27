import { expect, apiRequest, test } from "./helpers"

import {
  API,
  cleanupFood,
  cleanupSlot,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Plate feedback + AI memory loop", () => {
  test("mark a plate loved, then a new conversation's system prompt sees the tag", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.fb-${tag}`, "Moon", 999)
    const ing = await seedLeafFood({ name: `Chicken ${tag}`, kcal_100g: 100 })
    const comp = await seedComposedFood({
      name: `Curry ${tag}`,
      role: "main",
      children: [
        {
          child_id: ing.id,
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

      // Seed a plate through the UI — open the picker sheet, search, click food.
      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet = page.getByRole("dialog")
      await expect(sheet).toBeVisible()
      await sheet.getByRole("textbox").fill(`Curry ${tag}`)
      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await sheet
        .getByRole("button", { name: new RegExp(`Curry ${tag}`) })
        .click()
      await createPlateResp
      await expect(sheet).not.toBeVisible()
      await expect(cell.getByText(`Curry ${tag}`)).toBeVisible()

      // Click "Loved" on the slot card. Wait for the PUT feedback response.
      await cell.hover()
      const lovedBtn = cell.getByTestId("slot-action-love")
      const feedbackResp = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+\/feedback$/.test(r.url()) &&
          r.request().method() === "PUT"
      )
      await lovedBtn.click()
      await feedbackResp

      // The pressed state flips on.
      await expect(lovedBtn).toHaveAttribute("aria-pressed", "true")

      // Fetch the debug endpoint directly (dev-mode only) and assert the
      // component's tag landed in the profile preferences section of the
      // system prompt. This is the full round-trip: feedback → profile →
      // ComposePrompt.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const prompt = await ctx.get("/api/ai/debug/system-prompt")
      expect(prompt.ok()).toBeTruthy()
      const body = (await prompt.json()) as { system_prompt: string }
      expect(body.system_prompt).toContain(`spicy-${tag}`)
      await ctx.dispose()
    } finally {
      await cleanupFood(comp.id)
      await cleanupFood(ing.id)
      await cleanupSlot(slot.id)
    }
  })

  test("clicking the active status clears feedback", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.fb2-${tag}`, "Moon", 998)
    const ing = await seedLeafFood({ name: `Rice ${tag}`, kcal_100g: 100 })
    const comp = await seedComposedFood({
      name: `Bowl ${tag}`,
      role: "main",
      children: [
        {
          child_id: ing.id,
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

      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet2 = page.getByRole("dialog")
      await expect(sheet2).toBeVisible()
      await sheet2.getByRole("textbox").fill(`Bowl ${tag}`)
      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await sheet2
        .getByRole("button", { name: new RegExp(`Bowl ${tag}`) })
        .click()
      await createPlateResp

      await cell.hover()
      const lovedBtn = cell.getByTestId("slot-action-love")

      const put = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+\/feedback$/.test(r.url()) &&
          r.request().method() === "PUT"
      )
      await lovedBtn.click()
      await put
      await expect(lovedBtn).toHaveAttribute("aria-pressed", "true")

      const del = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+\/feedback$/.test(r.url()) &&
          r.request().method() === "DELETE"
      )
      await lovedBtn.click()
      await del
      await expect(lovedBtn).toHaveAttribute("aria-pressed", "false")
    } finally {
      await cleanupFood(comp.id)
      await cleanupFood(ing.id)
      await cleanupSlot(slot.id)
    }
  })
})
