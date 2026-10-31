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
  seedComposedWithStub,
  seedSlot,
  stageFood,
  test,
  uid,
} from "./helpers"

// F4: batch flows in the plate sheet (UX §5.5, D29). Every test works on
// its own two slots in its own future window. The slots sort last, and the
// other slots' cells on the leftover day are No plate, so the server
// suggests this test's "Mon lunch" and "Mon dinner". A slot another spec
// adds meanwhile is plannable there too; `chooseOwnCells` unticks it.

interface Setup {
  tag: string
  start: string
  days: string[]
  lunch: number
  dinner: number
  chili: { id: number; name: string }
  cleanup: () => Promise<void>
}

async function setup(): Promise<Setup> {
  const tag = uid()
  const { start, days } = await futureWindow()
  const lunch = await seedSlot(`slot.pbt-lunch-${tag}`, "Sun", 99998)
  const dinner = await seedSlot(`slot.pbt-dinner-${tag}`, "Moon", 99999)
  const { composed: chili, stub } = await seedComposedWithStub(
    // keep_days 3: cooked on day 1, use by day 4.
    { name: `Chili ${tag}`, role: "main", reference_portions: 4, keep_days: 3 },
    tag
  )

  // Every other slot is No plate on day 2.
  const ctx = await apiRequest.newContext({ baseURL: API })
  const slotsRes = await ctx.get("/api/settings/slots?active=true")
  const { items } = (await slotsRes.json()) as { items: { id: number }[] }
  const blocked = items
    .map((s) => s.id)
    .filter((id) => id !== lunch.id && id !== dinner.id)
  for (const id of blocked) {
    await ctx.put(`/api/availability/${days[2]}/${id}`, {
      data: { mode: "none" },
    })
  }
  await ctx.dispose()

  return {
    tag,
    start,
    days,
    lunch: lunch.id,
    dinner: dinner.id,
    chili,
    cleanup: async () => {
      const c = await apiRequest.newContext({ baseURL: API })
      for (const id of blocked) {
        await c.delete(`/api/availability/${days[2]}/${id}`)
      }
      for (const date of days) {
        for (const id of [lunch.id, dinner.id]) {
          await c.delete(`/api/availability/${date}/${id}`)
        }
      }
      await c.dispose()
      await cleanupSlot(lunch.id)
      await cleanupSlot(dinner.id)
      await cleanupFood(chili.id)
      await cleanupFood(stub.id)
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

/** Ticks only this test's suggested cells, adding meals until both of
 *  its slots are suggested. */
async function chooseOwnCells(sheet: Locator, tag: string) {
  const labels = sheet.locator("label:has(input[type=checkbox])")
  for (let meals = 3; ; meals++) {
    await expect(labels).toHaveCount(meals - 1)
    if ((await labels.filter({ hasText: tag }).count()) === 2) break
    expect(meals, "own cells never suggested").toBeLessThan(7)
    await sheet.getByRole("button", { name: "Increase Meals" }).click()
  }
  for (const label of await labels.all()) {
    const box = label.getByRole("checkbox")
    await expect(box).toBeChecked()
    if (!((await label.textContent()) ?? "").includes(tag)) await box.uncheck()
  }
}

async function expectBatch(
  page: Page,
  s: Setup,
  cook: string,
  linked: [string, string]
) {
  await expect(planCell(page, s.days[1], s.dinner)).toContainText(cook)
  await expect(planCell(page, s.days[2], s.lunch)).toContainText(linked[0])
  await expect(planCell(page, s.days[2], s.dinner)).toContainText(linked[1])
}

test.describe("Plan batches", () => {
  test("Sunday batch by hand: cook as a batch, eaters, portions, a leftover and Remove batch", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)

      // F4-AC1: add Chili to "Sunday" dinner …
      let sheet = await openCellSheet(page, s.days[1], s.dinner)
      await sheet.getByRole("button", { name: "Add food" }).click()
      await stageFood(sheet, s.chili.name)
      await write(page, () => sheet.getByTestId("food-picker-commit").click())
      await expect(
        sheet.getByRole("heading", { name: s.chili.name })
      ).toBeVisible()
      await setEaters(page, sheet, 2)

      // … cook it as a batch of 3 meals, keeping both suggested cells.
      await sheet.getByRole("button", { name: "Cook as a batch" }).click()
      await sheet.getByRole("button", { name: "Increase Meals" }).click()
      await chooseOwnCells(sheet, s.tag)
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

      // F4-AC2: an exact batch follows the eaters (D11).
      sheet = await openCellSheet(page, s.days[1], s.dinner)
      await setEaters(page, sheet, 3)
      await expect(planCell(page, s.days[1], s.dinner)).toContainText(
        "Cook × 7"
      )
      await setEaters(page, sheet, 2)
      await expect(planCell(page, s.days[1], s.dinner)).toContainText(
        "Cook × 6"
      )

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
      sheet = await openCellSheet(page, s.days[1], s.dinner)
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
          slot_id: s.dinner,
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
      await expect(planCell(page, s.days[1], s.dinner)).toContainText(
        "Cook × 4"
      )

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
