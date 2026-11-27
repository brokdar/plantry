import type { Page } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// S1: rule-based substitutions without a key (HELP-3, HELP-7), at phone
// width. A Chili batch of 6 (recipe for 4: 400 g beans, 2 onions of 120 g)
// has its onion swapped for shallot; the leaf names are the synonym table's
// own, with the tag in parentheses (matching ignores a parenthetical).

const NUTRITION = {
  kcal_100g: 40,
  protein_100g: 1.1,
  fat_100g: 0.1,
  carbs_100g: 9.3,
}

interface Seed {
  start: string
  end: string
  sessionId: number
  onion: string
  cleanup: () => Promise<void>
}

async function seed(): Promise<Seed> {
  const tag = uid()
  const { start, days } = await futureWindow()
  const slot = await seedSlot(`slot.sub-${tag}`, "Moon", 1_000_000)
  const beans = await seedLeafFood({ name: `Kidney beans (${tag})` })
  const onion = await seedLeafFood({
    name: `Zwiebel (${tag})`,
    ...NUTRITION,
  })
  const shallot = await seedLeafFood({
    name: `Schalotte (${tag})`,
    ...NUTRITION,
  })
  const chili = await seedComposedFood({
    name: `Chili ${tag}`,
    role: "main",
    reference_portions: 4,
    children: [
      { child_id: beans.id, amount: 400, unit: "g", grams: 400, sort_order: 0 },
      {
        child_id: onion.id,
        amount: 2,
        unit: "piece",
        grams: 240,
        sort_order: 1,
      },
    ],
    instructions: [{ step_number: 1, text: "Chop the onions." }],
  })
  const ctx = await apiRequest.newContext({ baseURL: API })
  const plate = await ctx.post("/api/plates", {
    data: {
      date: days[1],
      slot_id: slot.id,
      components: [{ food_id: chili.id, portions: 1 }],
    },
  })
  expect(plate.ok(), await plate.text()).toBeTruthy()
  const body = (await plate.json()) as {
    components: { cook_session_id: number }[]
  }
  const sessionId = body.components[0].cook_session_id
  const put = await ctx.put(`/api/cook-sessions/${sessionId}`, {
    data: { portions: 6, frozen_portions: 0, note: null },
  })
  expect(put.ok(), await put.text()).toBeTruthy()
  await ctx.dispose()
  return {
    start,
    end: days[6],
    sessionId,
    onion: onion.name,
    cleanup: async () => {
      await cleanupSlot(slot.id)
      await cleanupFood(chili.id)
      await cleanupFood(beans.id)
      await cleanupFood(onion.id)
      await cleanupFood(shallot.id)
    },
  }
}

async function undo(page: Page) {
  await page.getByRole("button", { name: "Undo" }).click()
}

test.describe("Substitutions", () => {
  // AC-16, cook mode: amounts follow, Undo restores.
  test("cook mode: I don't have an ingredient, then Undo", async ({ page }) => {
    const s = await seed()
    try {
      await page.setViewportSize({ width: 375, height: 812 })
      await page.goto(`/cook/${s.sessionId}`)
      const ingredient = (name: string) =>
        page.getByTestId("cook-ingredient").filter({ hasText: name })
      await expect(ingredient("Zwiebel")).toContainText("3 pcs")

      await page.getByTestId("substitute-open").click()
      const dialog = page.getByTestId("substitute-dialog")
      await dialog
        .getByTestId("substitute-pick")
        .filter({ hasText: "Zwiebel" })
        .click()
      const option = dialog.getByTestId("substitute-option").first()
      await expect(option).toContainText(/360 g .*Schalotte/)
      await option.click()

      await expect(ingredient("Schalotte")).toContainText("360 g")
      await expect(ingredient("Zwiebel")).toHaveCount(0)
      await expect(ingredient("Kidney beans")).toContainText("600 g")

      await undo(page)
      await expect(ingredient("Zwiebel")).toContainText("3 pcs")
      await expect(ingredient("Schalotte")).toHaveCount(0)
    } finally {
      await s.cleanup()
    }
  })

  // AC-16, shop: Not available applies to the batch that needs it.
  test("shop: Not available, then Undo", async ({ page }) => {
    const s = await seed()
    try {
      await page.setViewportSize({ width: 375, height: 812 })
      const list = page.waitForResponse(
        (r) => r.url().includes("/api/shopping-list?") && r.ok()
      )
      await page.goto(`/shop?from=${s.start}&to=${s.end}`)
      await list
      const row = (name: string) =>
        page.getByTestId("shop-row").filter({ hasText: name })
      await expect(row("Zwiebel")).toBeVisible()

      await page.getByRole("button", { name: `Options for ${s.onion}` }).click()
      await page.getByRole("menuitem", { name: "Not available" }).click()
      await page.getByTestId("substitute-option").first().click()

      await expect(row("Schalotte")).toBeVisible()
      await expect(row("Zwiebel")).toHaveCount(0)

      await undo(page)
      await expect(row("Zwiebel")).toBeVisible()
      await expect(row("Schalotte")).toHaveCount(0)
    } finally {
      await s.cleanup()
    }
  })
})
