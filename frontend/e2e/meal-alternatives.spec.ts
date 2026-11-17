import type { Page } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  openCellSheet,
  planCell,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// Meal editing slice 5: the alternatives lists in the plate sheet. Plates
// are made through the API and unlocked into drafts (the alternatives work
// on drafts), in the test's own slot and future window.

interface PlanView {
  cells: {
    date: string
    slot_id: number
    plate: {
      id: number
      components: { food_id: number; food_name: string; role: string }[]
    } | null
  }[]
}

async function setup() {
  const tag = uid()
  const { start, days } = await futureWindow()
  const slot = await seedSlot(`slot.mal-${tag}`, "Moon", 99950)
  const base = await seedLeafFood({
    name: `Chickpeas ${tag}`,
    kcal_100g: 120,
    protein_100g: 8,
    fat_100g: 2,
    carbs_100g: 18,
  })
  const recipe = (
    name: string,
    role: string,
    extra: { prep_minutes?: number; cook_minutes?: number; portions?: number }
  ) =>
    seedComposedFood({
      name: `${name} ${tag}`,
      role,
      reference_portions: extra.portions ?? 1,
      prep_minutes: extra.prep_minutes,
      cook_minutes: extra.cook_minutes,
      children: [
        {
          child_id: base.id,
          amount: 250,
          unit: "g",
          grams: 250,
          sort_order: 0,
        },
      ],
    })
  const stew = await recipe("Stew", "main", {
    portions: 2,
    prep_minutes: 20,
    cook_minutes: 30,
  })
  const quick = await recipe("Quick", "main", {
    prep_minutes: 10,
    cook_minutes: 10,
  })
  const slow = await recipe("Slow", "main", {
    prep_minutes: 30,
    cook_minutes: 90,
  })
  const greens = await recipe("Greens", "side_veg", {
    prep_minutes: 5,
    cook_minutes: 5,
  })

  const ctx = await apiRequest.newContext({ baseURL: API })
  async function plate(date: string, components: object[]) {
    const res = await ctx.post("/api/plates", {
      data: { date, slot_id: slot.id, eaters: 1, components },
    })
    const body = (await res.json()) as { id: number }
    expect(res.ok(), JSON.stringify(body)).toBeTruthy()
    return body.id
  }
  // A batch: Stew cooks on day 0 and is the leftover on day 1.
  const cook = await plate(days[0], [{ food_id: stew.id, portions: 1 }])
  const plan = (await (await ctx.get(`/api/plan?start=${start}`)).json()) as {
    sessions: { id: number; food_id: number }[]
  }
  const session = plan.sessions.find((x) => x.food_id === stew.id)!
  await plate(days[1], [
    { food_id: stew.id, portions: 1, cook_session_id: session.id },
  ])
  // Slow main with a Greens side.
  const sided = await plate(days[2], [
    { food_id: slow.id, portions: 1 },
    { food_id: greens.id, portions: 1 },
  ])
  const unlock = await ctx.post(`/api/plates/${cook}/unlock`)
  expect(unlock.ok(), await unlock.text()).toBeTruthy()
  const unlockSided = await ctx.post(`/api/plates/${sided}/unlock`)
  expect(unlockSided.ok(), await unlockSided.text()).toBeTruthy()
  await ctx.dispose()

  return {
    start,
    days,
    slot: slot.id,
    stew,
    quick,
    slow,
    greens,
    plateAt: async (date: string) => {
      const c = await apiRequest.newContext({ baseURL: API })
      const view = (await (
        await c.get(`/api/plan?start=${start}`)
      ).json()) as PlanView
      await c.dispose()
      return view.cells.find((x) => x.date === date && x.slot_id === slot.id)
        ?.plate
    },
    cleanup: async () => {
      await cleanupSlot(slot.id)
      for (const f of [stew, quick, slow, greens]) await cleanupFood(f.id)
      await cleanupFood(base.id)
    },
  }
}

const rows = (page: Page) => page.getByTestId("alt-row")

test.describe("meal alternatives", () => {
  test("Something else on a leftover meal gives a different dish, with Undo", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      const cell = planCell(page, s.days[1], s.slot)
      await expect(cell).toContainText(s.stew.name)

      const sheet = await openCellSheet(page, s.days[1], s.slot)
      await sheet.getByRole("button", { name: "Something else" }).click()
      await expect(rows(page).first()).toBeVisible()
      await rows(page).first().click()

      await expect(page.getByTestId("plan-sheet")).toHaveCount(0)
      await expect(cell).not.toContainText(s.stew.name)
      const after = await s.plateAt(s.days[1])
      expect(after?.components.map((c) => c.food_id)).not.toContain(s.stew.id)
      expect(after?.components.length).toBeGreaterThan(0)

      const back = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/plan/revert") &&
          r.request().method() === "POST"
      )
      await page.getByRole("button", { name: "Undo", exact: true }).click()
      expect((await back).ok()).toBeTruthy()
      await expect(cell).toContainText(s.stew.name)
    } finally {
      await s.cleanup()
    }
  })

  test("Change main keeps the sides", async ({ page }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      const sheet = await openCellSheet(page, s.days[2], s.slot)
      await sheet.getByRole("button", { name: "Change main" }).click()
      await expect(rows(page).first()).toBeVisible()
      await rows(page).first().click()

      await expect(page.getByTestId("plan-sheet")).toHaveCount(0)
      await expect(planCell(page, s.days[2], s.slot)).not.toContainText(
        s.slow.name
      )
      const after = await s.plateAt(s.days[2])
      const foods = after?.components.map((c) => c.food_id) ?? []
      expect(foods).toContain(s.greens.id)
      expect(foods).not.toContain(s.slow.id)
    } finally {
      await s.cleanup()
    }
  })

  test("the Quicker chip keeps only dishes of 30 minutes or less", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      const sheet = await openCellSheet(page, s.days[2], s.slot)
      await sheet.getByRole("button", { name: "Change main" }).click()
      await expect(rows(page).first()).toBeVisible()

      const filtered = page.waitForRequest(
        (r) =>
          r.url().includes("/api/plan/alternatives") &&
          (r.postData() ?? "").includes('"max_minutes":30')
      )
      const quicker = page.getByRole("button", { name: "Quicker" })
      await quicker.click()
      await filtered
      await expect(quicker).toHaveAttribute("aria-pressed", "true")
      await expect(
        rows(page).filter({
          hasText: s.quick.name,
        })
      ).toHaveCount(1)
      await expect(rows(page).filter({ hasText: s.slow.name })).toHaveCount(0)
      for (const text of await rows(page).allInnerTexts()) {
        const minutes = /(\d+) min/.exec(text)
        if (minutes) expect(Number(minutes[1])).toBeLessThanOrEqual(30)
      }
    } finally {
      await s.cleanup()
    }
  })
})
