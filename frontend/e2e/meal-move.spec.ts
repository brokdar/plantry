import type { Locator, Page } from "@playwright/test"

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

// Meal editing slice 5: moving meals on the board. Each test seeds its own
// two slots and two plates in its own future window (Curry on day 1 dinner,
// Stew on day 2 dinner), moves them with the mouse or from the sheet, and
// undoes the move through the revert token.

const weekday = (iso: string) =>
  new Intl.DateTimeFormat("en", { weekday: "short", timeZone: "UTC" }).format(
    new Date(`${iso}T00:00:00Z`)
  )

async function setup() {
  const tag = uid()
  const { start, days } = await futureWindow()
  const lunch = await seedSlot(`slot.mm-lunch-${tag}`, "Sun", 99960)
  const dinner = await seedSlot(`slot.mm-dinner-${tag}`, "Moon", 99961)
  const base = await seedLeafFood({
    name: `Peas ${tag}`,
    kcal_100g: 80,
    protein_100g: 5,
    fat_100g: 1,
    carbs_100g: 12,
  })
  const recipe = (name: string) =>
    seedComposedFood({
      name: `${name} ${tag}`,
      role: "main",
      reference_portions: 1,
      children: [
        {
          child_id: base.id,
          amount: 300,
          unit: "g",
          grams: 300,
          sort_order: 0,
        },
      ],
    })
  const curry = await recipe("Curry")
  const stew = await recipe("Stew")
  const ctx = await apiRequest.newContext({ baseURL: API })
  const plate = async (date: string, food: number) => {
    const res = await ctx.post("/api/plates", {
      data: {
        date,
        slot_id: dinner.id,
        eaters: 1,
        components: [{ food_id: food, portions: 1 }],
      },
    })
    expect(res.ok(), await res.text()).toBeTruthy()
    return ((await res.json()) as { id: number }).id
  }
  await plate(days[1], curry.id)
  await plate(days[2], stew.id)
  await ctx.dispose()
  return {
    tag,
    start,
    days,
    lunch: lunch.id,
    dinner: dinner.id,
    curry: curry.name,
    stew: stew.name,
    cleanup: async () => {
      await cleanupSlot(lunch.id)
      await cleanupSlot(dinner.id)
      await cleanupFood(curry.id)
      await cleanupFood(stew.id)
      await cleanupFood(base.id)
    },
  }
}

/** Drags with the mouse in small steps, as dnd-kit's MouseSensor needs. */
async function drag(page: Page, from: Locator, to: Locator) {
  await from.scrollIntoViewIfNeeded()
  const a = await from.getByTestId("plan-cell").boundingBox()
  const b = await to.getByTestId("plan-cell").boundingBox()
  if (!a || !b) throw new Error("cell not visible")
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2)
  await page.mouse.down()
  await page.mouse.move(a.x + a.width / 2 + 10, a.y + a.height / 2, {
    steps: 3,
  })
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 12 })
  await expect(to).toHaveAttribute("data-over", "")
  await page.mouse.up()
}

/** A committed move (the dry run carries ?dry_run=1). */
const moved = (page: Page) =>
  page.waitForResponse(
    (r) =>
      /\/plates\/\d+\/move$/.test(r.url()) && r.request().method() === "POST"
  )
const reverted = (page: Page) =>
  page.waitForResponse(
    (r) =>
      r.url().endsWith("/api/plan/revert") && r.request().method() === "POST"
  )
const undo = (page: Page) =>
  page.getByRole("button", { name: "Undo", exact: true })

test.describe("meal move", () => {
  test("a drag to an empty cell moves the meal, and Undo puts it back", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      const from = planCell(page, s.days[1], s.dinner)
      const to = planCell(page, s.days[1], s.lunch)
      await expect(from).toContainText(s.curry)

      const done = moved(page)
      await drag(page, from, to)
      expect((await done).ok()).toBeTruthy()
      await expect(to).toContainText(s.curry)
      await expect(from).not.toContainText(s.curry)

      const back = reverted(page)
      await undo(page).click()
      expect((await back).ok()).toBeTruthy()
      await expect(from).toContainText(s.curry)
      await expect(to).not.toContainText(s.curry)
    } finally {
      await s.cleanup()
    }
  })

  test("a drag onto a taken cell offers Swap, and Undo swaps back", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      const a = planCell(page, s.days[1], s.dinner)
      const b = planCell(page, s.days[2], s.dinner)
      await expect(a).toContainText(s.curry)
      await expect(b).toContainText(s.stew)

      await drag(page, a, b)
      const menu = page.getByTestId("occupied-menu")
      await expect(menu).toBeVisible()
      for (const name of [
        "Swap",
        `Move ${s.stew} to…`,
        `Replace ${s.stew}`,
        "Cancel",
      ]) {
        await expect(menu.getByRole("menuitem", { name })).toBeVisible()
      }
      const done = moved(page)
      await menu.getByRole("menuitem", { name: "Swap" }).click()
      expect((await done).ok()).toBeTruthy()
      await expect(a).toContainText(s.stew)
      await expect(b).toContainText(s.curry)

      const back = reverted(page)
      await undo(page).click()
      expect((await back).ok()).toBeTruthy()
      await expect(a).toContainText(s.curry)
      await expect(b).toContainText(s.stew)
    } finally {
      await s.cleanup()
    }
  })

  test("Move to… in the sheet moves the meal to a free cell", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      const sheet = await openCellSheet(page, s.days[2], s.dinner)
      await sheet.getByRole("button", { name: "Move to…" }).click()
      const dialog = page.getByTestId("cell-choice")
      await expect(dialog).toBeVisible()
      const target = dialog.getByRole("button", {
        name: new RegExp(`^${weekday(s.days[3])} mm lunch ${s.tag}`, "i"),
      })
      const done = moved(page)
      await target.click()
      expect((await done).ok()).toBeTruthy()
      await expect(planCell(page, s.days[3], s.lunch)).toContainText(s.stew)
      await expect(planCell(page, s.days[2], s.dinner)).not.toContainText(
        s.stew
      )
    } finally {
      await s.cleanup()
    }
  })
})
