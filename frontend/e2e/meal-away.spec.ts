import type { Page } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  planCell,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// Meal editing slice 5: "Away all day" from the day header. A batch cooks
// on day 1 dinner and is eaten again on day 2 lunch; a Bowl is day 1 lunch.
// Marking day 1 away removes both meals of that day, the batch cooks on day
// 2 instead (the cook strip follows), the day is not flagged, and Undo
// brings it all back.

const long = (iso: string) =>
  new Intl.DateTimeFormat("en", { weekday: "long", timeZone: "UTC" }).format(
    new Date(`${iso}T00:00:00Z`)
  )
const short = (iso: string) =>
  new Intl.DateTimeFormat("en", { weekday: "short", timeZone: "UTC" }).format(
    new Date(`${iso}T00:00:00Z`)
  )

async function setup() {
  const tag = uid()
  const { start, days } = await futureWindow()
  const lunch = await seedSlot(`slot.ma-lunch-${tag}`, "Sun", 99940)
  const dinner = await seedSlot(`slot.ma-dinner-${tag}`, "Moon", 99941)
  const base = await seedLeafFood({
    name: `Lentils ${tag}`,
    kcal_100g: 110,
    protein_100g: 9,
    fat_100g: 1,
    carbs_100g: 18,
  })
  const recipe = (name: string, portions: number, keep?: number) =>
    seedComposedFood({
      name: `${name} ${tag}`,
      role: "main",
      reference_portions: portions,
      keep_days: keep,
      children: [
        {
          child_id: base.id,
          amount: 400,
          unit: "g",
          grams: 400,
          sort_order: 0,
        },
      ],
    })
  const stew = await recipe("Stew", 2, 3)
  const bowl = await recipe("Bowl", 1)

  const ctx = await apiRequest.newContext({ baseURL: API })
  async function plate(date: string, slot: number, component: object) {
    const res = await ctx.post("/api/plates", {
      data: { date, slot_id: slot, eaters: 1, components: [component] },
    })
    const body = (await res.json()) as { id: number }
    expect(res.ok(), JSON.stringify(body)).toBeTruthy()
    return body.id
  }
  const cook = await plate(days[1], dinner.id, {
    food_id: stew.id,
    portions: 1,
  })
  const plan = await (await ctx.get(`/api/plan?start=${start}`)).json()
  const session = (
    plan.sessions as { id: number; food_id: number; portions: number }[]
  ).find((x) => x.food_id === stew.id)!
  await plate(days[2], lunch.id, {
    food_id: stew.id,
    portions: 1,
    cook_session_id: session.id,
  })
  const bowlPlate = await plate(days[1], lunch.id, {
    food_id: bowl.id,
    portions: 1,
  })
  // Drafts, the way the generator leaves them: Away removes them directly.
  for (const id of [cook, bowlPlate]) {
    const res = await ctx.post(`/api/plates/${id}/unlock`)
    expect(res.ok(), await res.text()).toBeTruthy()
  }
  await ctx.dispose()

  return {
    start,
    days,
    lunch: lunch.id,
    dinner: dinner.id,
    stew: stew.name,
    bowl: bowl.name,
    cleanup: async () => {
      const c = await apiRequest.newContext({ baseURL: API })
      const view = await (await c.get(`/api/plan?start=${start}`)).json()
      for (const slot of view.slots as { id: number }[]) {
        await c.delete(`/api/availability/${days[1]}/${slot.id}`)
      }
      await c.dispose()
      await cleanupSlot(lunch.id)
      await cleanupSlot(dinner.id)
      await cleanupFood(stew.id)
      await cleanupFood(bowl.id)
      await cleanupFood(base.id)
    },
  }
}

const cookEntries = (page: Page, iso: string, food: string) =>
  page
    .getByRole("region", { name: "Cook days" })
    .getByRole("button", { name: new RegExp(`^${short(iso)} · ${food}`) })

test.describe("meal away", () => {
  test("Away all day removes the day's meals, moves the cook, flags nothing, and Undo restores it", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      const dinner = planCell(page, s.days[1], s.dinner)
      const lunch = planCell(page, s.days[1], s.lunch)
      const nextLunch = planCell(page, s.days[2], s.lunch)
      await expect(dinner).toContainText(s.stew)
      await expect(lunch).toContainText(s.bowl)
      await expect(cookEntries(page, s.days[1], s.stew)).toHaveCount(1)
      await expect(cookEntries(page, s.days[2], s.stew)).toHaveCount(0)

      await page
        .getByRole("button", {
          name: `Actions for ${long(s.days[1])} ${Number(s.days[1].slice(8))}`,
        })
        .click()
      await page.getByRole("menuitem", { name: "Away all day" }).click()
      // The batch will cook on day 2: the preview says so before committing.
      const preview = page.getByTestId("also-changed")
      await expect(preview).toContainText("will now be cooked")
      const away = page.waitForResponse(
        (r) =>
          r.url().includes("/api/availability/away") &&
          !r.url().includes("dry_run") &&
          r.request().method() === "POST"
      )
      await preview.getByRole("button", { name: "Confirm" }).click()
      expect((await away).ok()).toBeTruthy()

      await expect(dinner).not.toContainText(s.stew)
      await expect(lunch).not.toContainText(s.bowl)
      await expect(dinner).toContainText("No plate")
      await expect(lunch).toContainText("No plate")
      // The batch cooks on day 2 now, and day 1 no longer cooks.
      await expect(cookEntries(page, s.days[1], s.stew)).toHaveCount(0)
      await expect(cookEntries(page, s.days[2], s.stew)).toHaveCount(1)
      await expect(nextLunch).toContainText("Cook")
      // The away day is not flagged: no cell of it, empty or not, "needs
      // attention" (an away day has no kcal floor to miss).
      await expect(
        page.locator(
          `[data-cell-key^="${s.days[1]}|"] [aria-label*="needs attention"]`
        )
      ).toHaveCount(0)

      const back = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/plan/revert") &&
          r.request().method() === "POST"
      )
      await page.getByRole("button", { name: "Undo", exact: true }).click()
      const reverted = await back
      expect(reverted.ok(), await reverted.text()).toBeTruthy()
      await expect(dinner).toContainText(s.stew)
      await expect(lunch).toContainText(s.bowl)
      await expect(cookEntries(page, s.days[1], s.stew)).toHaveCount(1)
      await expect(cookEntries(page, s.days[2], s.stew)).toHaveCount(0)
    } finally {
      await s.cleanup()
    }
  })
})
