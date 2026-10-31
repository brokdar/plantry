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
  stageFood,
  test,
  uid,
} from "./helpers"

// F4: the plate sheet, the cell sheet and the food picker on /plan. Each
// test seeds its own slot and works in its own future window.

interface Setup {
  tag: string
  start: string
  days: string[]
  slot: number
  /** 300 kcal per portion (300 g of a 100 kcal/100 g leaf, 1 portion). */
  bowl: { id: number; name: string }
  /** Leaf, 52 kcal/100 g. */
  apple: { id: number; name: string }
  cleanup: () => Promise<void>
}

async function setup(): Promise<Setup> {
  const tag = uid()
  const { start, days } = await futureWindow()
  const slot = await seedSlot(`slot.ps-${tag}`, "Moon", 99990)
  const base = await seedLeafFood({
    name: `Grain ${tag}`,
    kcal_100g: 100,
    protein_100g: 10,
    fat_100g: 2,
    carbs_100g: 15,
  })
  const bowl = await seedComposedFood({
    name: `Bowl ${tag}`,
    role: "main",
    reference_portions: 1,
    children: [
      { child_id: base.id, amount: 300, unit: "g", grams: 300, sort_order: 0 },
    ],
  })
  const apple = await seedLeafFood({
    name: `Apple ${tag}`,
    kcal_100g: 52,
    protein_100g: 0.3,
    fat_100g: 0.2,
    carbs_100g: 14,
  })
  return {
    tag,
    start,
    days,
    slot: slot.id,
    bowl,
    apple,
    cleanup: async () => {
      const ctx = await apiRequest.newContext({ baseURL: API })
      for (const date of days) {
        await ctx.delete(`/api/availability/${date}/${slot.id}`)
      }
      await ctx.dispose()
      await cleanupSlot(slot.id)
      await cleanupFood(bowl.id)
      await cleanupFood(base.id)
      await cleanupFood(apple.id)
    },
  }
}

async function seedPlate(
  date: string,
  slotId: number,
  eaters: number,
  components: object[]
): Promise<number> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId, eaters, components },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  const { id } = (await res.json()) as { id: number }
  await ctx.dispose()
  return id
}

function waitForWrite(page: Page, path: RegExp, method: string) {
  return page.waitForResponse(
    (r) => path.test(r.url()) && r.request().method() === method && r.ok()
  )
}

test.describe("Plate sheet", () => {
  // F4-AC7
  test("the food picker stages two foods and creates the plate in one request", async ({
    page,
  }) => {
    const s = await setup()
    try {
      const posts: string[] = []
      page.on("request", (r) => {
        if (r.method() === "POST" && /\/api\/plates/.test(r.url())) {
          posts.push(r.url())
        }
      })
      await page.goto(`/plan?start=${s.start}`)
      const sheet = await openCellSheet(page, s.days[0], s.slot)
      await sheet.getByRole("button", { name: "Add food" }).click()
      await stageFood(sheet, s.bowl.name)
      await stageFood(sheet, s.apple.name, "Ingredients")
      await expect(sheet.getByTestId("food-picker-staged-row")).toHaveCount(2)

      const created = waitForWrite(page, /\/api\/plates$/, "POST")
      await sheet.getByRole("button", { name: "Add 2 foods" }).click()
      await created
      const cell = planCell(page, s.days[0], s.slot)
      await expect(cell).toContainText(s.bowl.name)
      await expect(cell).toContainText(s.apple.name)
      expect(posts).toHaveLength(1)
    } finally {
      await s.cleanup()
    }
  })

  // F4-AC8 (replaces plate-quantity and plate-macros)
  test("leaf amounts and recipe portions round-trip; kcal are for one eater", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await seedPlate(s.days[0], s.slot, 2, [
        { food_id: s.bowl.id, portions: 1 },
      ])
      await seedPlate(s.days[1], s.slot, 2, [
        { food_id: s.apple.id, amount: 100, unit: "g" },
      ])
      await page.goto(`/plan?start=${s.start}`)

      // The recipe: 300 kcal for one eater, whatever the eaters.
      let sheet = await openCellSheet(page, s.days[0], s.slot)
      const kcal = sheet.getByTestId("sheet-kcal")
      await expect(kcal).toHaveText("300 kcal")
      let put = waitForWrite(page, /\/api\/plates\/\d+$/, "PUT")
      await sheet.getByRole("button", { name: "Increase Eaters" }).click()
      await put
      await expect(sheet.getByRole("group", { name: "Eaters" })).toContainText(
        "2½"
      )
      await expect(kcal).toHaveText("300 kcal")

      // 1.5 portions per eater.
      const portions = sheet.getByRole("group", {
        name: `Portions of ${s.bowl.name} per eater`,
      })
      for (const shown of ["1¼", "1½"]) {
        put = waitForWrite(page, /\/components\/\d+$/, "PUT")
        await portions.getByRole("button", { name: /^Increase/ }).click()
        await put
        await expect(portions.locator("output")).toHaveText(shown)
      }
      await expect(kcal).toHaveText("450 kcal")

      // The leaf: 150 g apple.
      await page.keyboard.press("Escape")
      sheet = await openCellSheet(page, s.days[1], s.slot)
      const amount = sheet.getByTestId("quantity-unit-amount")
      await expect(amount).toHaveValue("100")
      await amount.fill("150")
      put = waitForWrite(page, /\/components\/\d+$/, "PUT")
      await sheet.getByRole("button", { name: "Update amount" }).click()
      await put
      await expect(sheet.getByTestId("sheet-kcal")).toHaveText("78 kcal")

      await page.reload()
      sheet = await openCellSheet(page, s.days[1], s.slot)
      await expect(sheet.getByTestId("quantity-unit-amount")).toHaveValue("150")
      await page.keyboard.press("Escape")
      sheet = await openCellSheet(page, s.days[0], s.slot)
      await expect(
        sheet
          .getByRole("group", { name: `Portions of ${s.bowl.name} per eater` })
          .locator("output")
      ).toHaveText("1½")
    } finally {
      await s.cleanup()
    }
  })

  // F4-AC9 (replaces feedback.spec)
  test("feedback writes loved, disliked and not_eaten; tapping the active one clears it", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await seedPlate(s.days[0], s.slot, 2, [
        { food_id: s.bowl.id, portions: 1 },
      ])
      await page.goto(`/plan?start=${s.start}`)
      const sheet = await openCellSheet(page, s.days[0], s.slot)

      for (const [label, status] of [
        ["Loved", "loved"],
        ["Not again", "disliked"],
        ["Didn't eat it", "not_eaten"],
      ] as const) {
        const req = page.waitForRequest(
          (r) => r.url().endsWith("/feedback") && r.method() === "PUT"
        )
        await sheet.getByRole("button", { name: label }).click()
        expect((await req).postDataJSON()).toEqual({ status })
        await expect(
          sheet.getByRole("button", { name: label })
        ).toHaveAttribute("aria-pressed", "true")
      }

      const cleared = waitForWrite(page, /\/feedback$/, "DELETE")
      await sheet.getByRole("button", { name: "Didn't eat it" }).click()
      await cleared
      await expect(
        sheet.getByRole("button", { name: "Didn't eat it" })
      ).toHaveAttribute("aria-pressed", "false")
    } finally {
      await s.cleanup()
    }
  })

  test("Save as routine, then Use a routine on another day", async ({
    page,
  }) => {
    const s = await setup()
    let presetId: number | undefined
    try {
      await seedPlate(s.days[0], s.slot, 2, [
        { food_id: s.bowl.id, portions: 1 },
      ])
      await page.goto(`/plan?start=${s.start}`)
      let sheet = await openCellSheet(page, s.days[0], s.slot)
      await sheet.getByRole("button", { name: "Save as routine" }).click()
      const routine = `Routine ${s.tag}`
      await sheet.getByTestId("sheet-routine-name").fill(routine)
      const created = waitForWrite(page, /\/api\/presets$/, "POST")
      await sheet.getByRole("button", { name: "Save as routine" }).click()
      presetId = ((await (await created).json()) as { id: number }).id
      await expect(
        page.getByText(`Saved as routine “${routine}”`)
      ).toBeVisible()
      await page.keyboard.press("Escape")

      sheet = await openCellSheet(page, s.days[2], s.slot)
      const applied = waitForWrite(page, /\/apply$/, "POST")
      await sheet.getByRole("button", { name: routine }).click()
      await applied
      await expect(planCell(page, s.days[2], s.slot)).toContainText(s.bowl.name)
    } finally {
      if (presetId !== undefined) {
        const ctx = await apiRequest.newContext({ baseURL: API })
        await ctx.delete(`/api/presets/${presetId}`)
        await ctx.dispose()
      }
      await s.cleanup()
    }
  })

  test("a No plate cell keeps a note and can be planned again", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.goto(`/plan?start=${s.start}`)
      let sheet = await openCellSheet(page, s.days[4], s.slot)
      await sheet.getByRole("button", { name: "Mark as No plate" }).click()
      const cell = planCell(page, s.days[4], s.slot)
      await expect(cell).toContainText("No plate")

      sheet = await openCellSheet(page, s.days[4], s.slot)
      await sheet.getByTestId("cell-note").fill("Eating out")
      await sheet.getByRole("button", { name: "Save note" }).click()
      await expect(cell).toContainText("Eating out")

      await sheet.getByRole("button", { name: "Plan this meal" }).click()
      await expect(cell).toContainText("Add")
    } finally {
      await s.cleanup()
    }
  })

  test("the sheet is a bottom sheet on phones", async ({ page }) => {
    const s = await setup()
    try {
      await seedPlate(s.days[0], s.slot, 2, [
        { food_id: s.bowl.id, portions: 1 },
      ])
      await page.setViewportSize({ width: 375, height: 800 })
      await page.goto(`/plan?start=${s.start}`)
      const sheet = await openCellSheet(page, s.days[0], s.slot)
      await expect(sheet).toHaveAttribute("data-side", "bottom")
      await expect(sheet.getByTestId("sheet-kcal")).toHaveText("300 kcal")
    } finally {
      await s.cleanup()
    }
  })
})
