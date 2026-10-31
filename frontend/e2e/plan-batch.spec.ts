import type { APIRequestContext, Locator, Page } from "@playwright/test"

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
  stageFood,
  test,
  uid,
} from "./helpers"

// F4: batch flows in the plate sheet (UX §5.5, D29). Every test works on
// its own slots in its own future window (`futureWindow`). "Sunday dinner"
// is a slot that sorts last, so it cooks after every other meal of its day;
// "Monday lunch" and "Monday dinner" sort first, before every other slot,
// so the server suggests them as the next two leftover cells. The cook
// day's other cells are No plate (`blockOthers`), so a slot another spec
// adds that sorts even later cannot come between. The tests run one after
// the other so they never suggest each other's slots.
test.describe.configure({ mode: "serial" })

interface Setup {
  tag: string
  start: string
  days: string[]
  /** The cook cell's slot ("Sunday dinner"); sorts last. */
  cook: number
  /** The leftover slots ("Monday lunch", "Monday dinner"); sort first. */
  lunch: number
  dinner: number
  chili: { id: number; name: string }
  /** Makes this test's Mon lunch and Mon dinner the batch's next two
   *  leftover cells: every other slot is No plate on the cook day. */
  blockOthers: () => Promise<void>
  cleanup: () => Promise<void>
}

async function setup(): Promise<Setup> {
  const tag = uid()
  const { start, days } = await futureWindow()
  const lunch = await seedSlot(`slot.pbt-lunch-${tag}`, "Sun", -2)
  const dinner = await seedSlot(`slot.pbt-dinner-${tag}`, "Moon", -1)
  const cook = await seedSlot(`slot.pbt-cook-${tag}`, "Moon", 1_000_000)
  const own = new Set([cook.id, lunch.id, dinner.id])
  // 400 g at 100 kcal/100 g for 4 portions: 100 kcal a portion.
  const beans = await seedLeafFood({
    name: `Beans ${tag}`,
    kcal_100g: 100,
    protein_100g: 8,
    fat_100g: 1,
    carbs_100g: 15,
  })
  const chili = await seedComposedFood({
    name: `Chili ${tag}`,
    role: "main",
    reference_portions: 4,
    // Cooked on day 1, use by day 4.
    keep_days: 3,
    children: [
      { child_id: beans.id, amount: 400, unit: "g", grams: 400, sort_order: 0 },
    ],
  })

  // Cells blocked for this test, as `${date}/${slotId}`.
  const blocked = new Set<string>()
  async function block(ctx: APIRequestContext, date: string, id: number) {
    const cell = `${date}/${id}`
    if (blocked.has(cell)) return
    blocked.add(cell)
    await ctx.put(`/api/availability/${cell}`, { data: { mode: "none" } })
  }

  async function blockOthers() {
    const ctx = await apiRequest.newContext({ baseURL: API })
    const res = await ctx.get("/api/settings/slots?active=true")
    const { items } = (await res.json()) as { items: { id: number }[] }
    for (const { id } of items) {
      if (!own.has(id)) await block(ctx, days[1], id)
    }
    // A slot another spec adds meanwhile is plannable again: ask the server
    // what it would suggest and block until only this test's cells remain.
    const planRes = await ctx.get(`/api/plan?start=${start}`)
    const { sessions } = (await planRes.json()) as {
      sessions: { id: number; food_id: number }[]
    }
    const session = sessions.find((x) => x.food_id === chili.id)
    expect(session, "the fresh Chili session").toBeTruthy()
    for (let attempt = 0; attempt < 10; attempt++) {
      const cellsRes = await ctx.get(
        `/api/cook-sessions/${session!.id}/leftover-cells?count=2`
      )
      const { cells } = (await cellsRes.json()) as {
        cells: { date: string; slot_id: number }[]
      }
      const foreign = cells.filter(
        (c) => c.slot_id !== lunch.id && c.slot_id !== dinner.id
      )
      if (foreign.length === 0) break
      for (const c of foreign) await block(ctx, c.date, c.slot_id)
    }
    await ctx.dispose()
  }

  return {
    tag,
    start,
    days,
    cook: cook.id,
    lunch: lunch.id,
    dinner: dinner.id,
    chili,
    blockOthers,
    cleanup: async () => {
      const c = await apiRequest.newContext({ baseURL: API })
      for (const cell of blocked) {
        await c.delete(`/api/availability/${cell}`)
      }
      for (const date of days) {
        for (const id of own) {
          await c.delete(`/api/availability/${date}/${id}`)
        }
      }
      await c.dispose()
      for (const id of own) await cleanupSlot(id)
      await cleanupFood(chili.id)
      await cleanupFood(beans.id)
    },
  }
}

const strip = (page: Page) => page.getByRole("region", { name: "Cook days" })

/** Waits for the next session or plate write and the plan refetch. */
async function write(page: Page, action: () => Promise<void>) {
  const done = page.waitForResponse(
    (r) =>
      /\/api\/(cook-sessions|plates)/.test(r.url()) &&
      r.request().method() !== "GET"
  )
  await action()
  expect((await done).ok()).toBeTruthy()
}

/** "2½" → 2.5. */
function parseAmount(text: string): number {
  const glyphs: Record<string, number> = { "¼": 0.25, "½": 0.5, "¾": 0.75 }
  const glyph = text.at(-1) ?? ""
  return glyph in glyphs
    ? Number(text.slice(0, -1) || "0") + glyphs[glyph]
    : Number(text)
}

/** Steps a plate sheet's Eaters to `target`. New plates, leftovers
 *  included, get the household's eaters (D10, D29), which other specs may
 *  set; AC-1 is for a household of two. */
async function setEaters(page: Page, sheet: Locator, target: number) {
  const group = sheet.getByRole("group", { name: "Eaters" })
  for (let i = 0; i < 40; i++) {
    const text = (await group.locator("output").textContent()) ?? ""
    const shown = parseAmount(text)
    if (shown === target) return
    const name = shown < target ? "Increase Eaters" : "Decrease Eaters"
    await write(page, () => sheet.getByRole("button", { name }).click())
    await expect(group.locator("output")).not.toHaveText(text)
  }
  throw new Error(`eaters never reached ${target}`)
}

/**
 * Opens Cook as a batch on the cook cell with 3 meals, so the two suggested
 * leftover cells are this test's Mon lunch and Mon dinner. A slot another
 * spec adds after `blockOthers` is plannable on the leftover day and comes
 * first; then block again and ask anew on a fresh page.
 */
async function openBatchForm(page: Page, s: Setup): Promise<Locator> {
  for (let attempt = 0; ; attempt++) {
    await s.blockOthers()
    await page.goto(`/plan?start=${s.start}`)
    const sheet = await openCellSheet(page, s.days[1], s.cook)
    await sheet.getByRole("button", { name: "Cook as a batch" }).click()
    const suggested = page.waitForResponse(
      (r) => /\/leftover-cells\?count=2$/.test(r.url()) && r.ok()
    )
    await sheet.getByRole("button", { name: "Increase Meals" }).click()
    await suggested
    const labels = sheet.locator("label:has(input[type=checkbox])")
    await expect(labels).toHaveCount(2)
    if ((await labels.filter({ hasText: s.tag }).count()) === 2) {
      for (const box of await labels.getByRole("checkbox").all()) {
        await expect(box).toBeChecked()
      }
      return sheet
    }
    expect(attempt, "own cells never suggested").toBeLessThan(4)
  }
}

async function expectBatch(
  page: Page,
  s: Setup,
  cook: string,
  linked: [string, string]
) {
  await expect(planCell(page, s.days[1], s.cook)).toContainText(cook)
  await expect(planCell(page, s.days[2], s.lunch)).toContainText(linked[0])
  await expect(planCell(page, s.days[2], s.dinner)).toContainText(linked[1])
}

test.describe("Plan batches", () => {
  test("Sunday batch by hand: cook as a batch, eaters, portions, a leftover and Remove batch", async ({
    page,
  }) => {
    test.slow()
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)

      // F4-AC1: add Chili to "Sunday" dinner …
      let sheet = await openCellSheet(page, s.days[1], s.cook)
      await sheet.getByRole("button", { name: "Add food" }).click()
      await stageFood(sheet, s.chili.name)
      await write(page, () => sheet.getByTestId("food-picker-commit").click())
      await expect(
        sheet.getByRole("heading", { name: s.chili.name })
      ).toBeVisible()
      await setEaters(page, sheet, 2)

      // … cook it as a batch of 3 meals, keeping both suggested cells.
      sheet = await openBatchForm(page, s)
      await write(page, () =>
        sheet.getByRole("button", { name: "Create batch" }).click()
      )
      await page.keyboard.press("Escape")
      for (const slot of [s.lunch, s.dinner]) {
        sheet = await openCellSheet(page, s.days[2], slot)
        await setEaters(page, sheet, 2)
        await page.keyboard.press("Escape")
      }
      await expectBatch(page, s, "Cook × 6", ["2/3", "3/3"])
      await page.reload()
      await expectBatch(page, s, "Cook × 6", ["2/3", "3/3"])

      // AC-1 step 3: Monday's meter is for one eater: two Chili portions
      // (lunch and dinner) at 100 kcal each, not four.
      const monday = new Intl.DateTimeFormat("en-US", {
        weekday: "long",
        month: "long",
        day: "numeric",
      }).format(new Date(`${s.days[2]}T12:00:00`))
      await page.getByRole("button", { name: `Nutrition, ${monday}` }).click()
      const nutrition = page.getByRole("dialog", { name: monday })
      await expect(nutrition).toContainText("Per person")
      await expect(nutrition.getByRole("definition").first()).toHaveText(
        /^200\b/
      )
      await page.keyboard.press("Escape")

      // F4-AC2: an exact batch follows the eaters (D11).
      sheet = await openCellSheet(page, s.days[1], s.cook)
      await setEaters(page, sheet, 3)
      await expect(planCell(page, s.days[1], s.cook)).toContainText("Cook × 7")
      await setEaters(page, sheet, 2)
      await expect(planCell(page, s.days[1], s.cook)).toContainText("Cook × 6")

      // F4-AC3: 8 portions leave 2 spare.
      const portions = sheet.getByRole("group", { name: "Portions to cook" })
      for (const n of ["7", "8"]) {
        await write(page, () =>
          sheet
            .getByRole("button", { name: "Increase Portions to cook" })
            .click()
        )
        await expect(portions.locator("output")).toHaveText(n)
      }
      await page.keyboard.press("Escape")
      await expect(strip(page)).toContainText("2 spare")

      // F4-AC5: past use-by (day 6), the batch is not on offer …
      sheet = await openCellSheet(page, s.days[6], s.lunch)
      await expect(
        sheet.getByRole("button", { name: "Add food" })
      ).toBeVisible()
      await expect(sheet.getByTestId("cell-leftover")).toHaveCount(0)
      await page.keyboard.press("Escape")

      // … within it (day 3 lunch) it is, and eating it uses the spare.
      sheet = await openCellSheet(page, s.days[3], s.lunch)
      const leftover = sheet.getByTestId("cell-leftover")
      await expect(leftover).toContainText(s.chili.name)
      await write(page, () => leftover.click())
      await expect(planCell(page, s.days[3], s.lunch)).toContainText("4/4")
      sheet = await openCellSheet(page, s.days[3], s.lunch)
      await setEaters(page, sheet, 2)
      await page.keyboard.press("Escape")
      await expect(strip(page)).not.toContainText("spare")

      // F4-AC4: Remove batch asks first, removes the leftovers, and Undo
      // brings all of them back with their links.
      sheet = await openCellSheet(page, s.days[1], s.cook)
      await sheet.getByRole("button", { name: "Remove batch" }).click()
      const confirm = page.getByRole("dialog", { name: "Remove this batch?" })
      await expect(confirm).toContainText("3 other plates")
      const removed = page.waitForResponse(
        (r) =>
          /\/api\/cook-sessions\/\d+$/.test(r.url()) &&
          r.request().method() === "DELETE"
      )
      await confirm.getByRole("button", { name: "Remove batch" }).click()
      expect((await removed).ok()).toBeTruthy()
      await expect(page.getByTestId("plan-sheet")).toHaveCount(0)
      for (const [day, slot] of [
        [2, s.lunch],
        [2, s.dinner],
        [3, s.lunch],
      ] as const) {
        await expect(planCell(page, s.days[day], slot)).toContainText("Add")
      }

      const restored = page.waitForResponse(
        (r) => r.url().endsWith("/api/plan/restore") && r.ok()
      )
      await page.getByRole("button", { name: "Undo", exact: true }).click()
      await restored
      await expectBatch(page, s, "Cook × 8", ["2/4", "3/4"])
      await expect(planCell(page, s.days[3], s.lunch)).toContainText("4/4")
    } finally {
      await s.cleanup()
    }
  })

  test("Mark as No plate on a leftover, then Undo, keeps its batch link", async ({
    page,
  }) => {
    const s = await setup()
    try {
      // The AC-1 batch through the API.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const plateRes = await ctx.post("/api/plates", {
        data: {
          date: s.days[1],
          slot_id: s.cook,
          eaters: 2,
          components: [{ food_id: s.chili.id, portions: 1 }],
        },
      })
      expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
      const plate = (await plateRes.json()) as {
        components: { cook_session_id: number }[]
      }
      const res = await ctx.post(
        `/api/cook-sessions/${plate.components[0].cook_session_id}/leftovers`,
        {
          data: {
            cells: [
              { date: s.days[2], slot_id: s.lunch },
              { date: s.days[2], slot_id: s.dinner },
            ],
          },
        }
      )
      expect(res.ok(), await res.text()).toBeTruthy()
      // Leftover plates get the household's eaters; AC-1 eats for two.
      const { links } = (await res.json()) as { links: { plate_id: number }[] }
      for (const { plate_id } of links.slice(1)) {
        const put = await ctx.put(`/api/plates/${plate_id}`, {
          data: { eaters: 2 },
        })
        expect(put.ok(), await put.text()).toBeTruthy()
      }
      await ctx.dispose()

      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      await expectBatch(page, s, "Cook × 6", ["2/3", "3/3"])

      // F4-AC6
      const sheet = await openCellSheet(page, s.days[2], s.lunch)
      await expect(sheet).toContainText("(2 of 3)")
      await sheet.getByRole("button", { name: "Mark as No plate" }).click()
      await expect(planCell(page, s.days[2], s.lunch)).toContainText("No plate")
      await expect(planCell(page, s.days[1], s.cook)).toContainText("Cook × 4")

      const restored = page.waitForResponse(
        (r) => r.url().endsWith("/api/plan/restore") && r.ok()
      )
      await page.getByRole("button", { name: "Undo", exact: true }).click()
      await restored
      await expectBatch(page, s, "Cook × 6", ["2/3", "3/3"])
    } finally {
      await s.cleanup()
    }
  })
})
