import type { Page } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  expect,
  futureWindow,
  seedComposedFood,
  seedLeafFood,
  test,
  uid,
} from "./helpers"

// The week ledger above the plan board: a seeded week (three complete days
// far under the kcal target) shows the strip with the kcal line
// highlighted, and a light day shows its mark. The profile's kcal target is
// global, so this spec runs in the global-settings project.

interface RawPlan {
  cells: {
    date: string
    slot_id: number
    mode: string
    plate: { id: number } | null
  }[]
}

test.describe("week ledger", () => {
  let start = ""
  let days: string[] = []
  let plateIds: number[] = []
  let foodIds: number[] = []
  let kcalBefore: number | null = null

  test.beforeEach(async () => {
    const tag = uid()
    ;({ start, days } = await futureWindow())
    const ctx = await apiRequest.newContext({ baseURL: API })
    const profile = (await (await ctx.get("/api/profile")).json()) as {
      kcal_target: number | null
    }
    kcalBefore = profile.kcal_target
    expect(
      (await ctx.put("/api/profile", { data: { kcal_target: 2000 } })).ok()
    ).toBeTruthy()

    const leaf = await seedLeafFood({
      name: `Ledger leaf ${tag}`,
      kcal_100g: 200,
      protein_100g: 10,
      fat_100g: 8,
      carbs_100g: 20,
    })
    const meal = await seedComposedFood({
      name: `Ledger meal ${tag}`,
      role: "main",
      reference_portions: 1,
      children: [
        {
          child_id: leaf.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })
    foodIds = [meal.id, leaf.id]

    // One plate in every meal cell of the first three days: complete days.
    const plan = (await (
      await ctx.get(`/api/plan?start=${start}`)
    ).json()) as RawPlan
    plateIds = []
    for (const c of plan.cells) {
      if (!days.slice(0, 3).includes(c.date) || c.mode !== "plan" || c.plate) {
        continue
      }
      const res = await ctx.post("/api/plates", {
        data: {
          date: c.date,
          slot_id: c.slot_id,
          components: [{ food_id: meal.id, portions: 1 }],
        },
      })
      expect(res.ok(), await res.text()).toBeTruthy()
      plateIds.push(((await res.json()) as { id: number }).id)
    }
    await ctx.dispose()
  })

  test.afterEach(async () => {
    const ctx = await apiRequest.newContext({ baseURL: API })
    for (const id of plateIds) await ctx.delete(`/api/plates/${id}`)
    await ctx.put("/api/profile", { data: { kcal_target: kcalBefore } })
    await ctx.dispose()
    for (const id of foodIds) await cleanupFood(id)
  })

  async function open(page: Page) {
    await page.goto(`/plan?start=${start}`)
  }

  test("a seeded week shows the ledger, the kcal line highlighted", async ({
    page,
  }) => {
    await open(page)
    const ledger = page.getByTestId("week-ledger")
    await expect(ledger).toBeVisible()
    const kcal = ledger.getByTestId("ledger-kcal")
    await expect(kcal).toHaveAttribute("data-status", "low")
    await expect(kcal).toContainText("below")
    await expect(kcal).toContainText("kcal a day")
    await expect(ledger.getByTestId("ledger-carbs")).toBeVisible()
  })

  test("a light day shows its mark", async ({ page }) => {
    // Light requests come from the planner and the assistant, never a
    // request of the board: mark one day in the real response.
    await page.route("**/api/plan?*", async (route) => {
      const res = await route.fetch()
      const body = (await res.json()) as {
        days: { date: string; light?: string }[]
      }
      body.days = body.days.map((d) =>
        d.date === days[1] ? { ...d, light: "day" } : d
      )
      await route.fulfill({ response: res, json: body })
    })
    await open(page)
    await expect(page.getByTestId("week-ledger")).toBeVisible()
    const marks = page.getByTestId("light-mark")
    await expect(marks).toHaveCount(1)
    await expect(marks).toContainText("Light day")
  })
})
