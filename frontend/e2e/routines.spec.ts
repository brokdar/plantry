import type { Page } from "@playwright/test"

import {
  API,
  addDays,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  openPalette,
  planCell,
  seedLeafFood,
  seedPlateWithComponent,
  seedSlot,
  test,
  uid,
} from "./helpers"

function waitForWrite(page: Page, path: RegExp, method: string) {
  return page.waitForResponse(
    (r) => path.test(r.url()) && r.request().method() === method && r.ok()
  )
}

/** Deletes every routine whose name contains `tag` (ids get reused, so
 *  never delete by a remembered id). */
async function cleanupRoutines(tag: string) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(`/api/presets?search=${encodeURIComponent(tag)}`)
  const body = (await res.json()) as { items: { id: number; name: string }[] }
  for (const p of body.items) {
    if (p.name.includes(tag)) await ctx.delete(`/api/presets/${p.id}`)
  }
  await ctx.dispose()
}

async function seedRoutine(name: string, slotId: number, foodId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/presets", {
    data: {
      name,
      plates: [
        {
          slot_id: slotId,
          components: [{ food_id: foodId, amount: 120, unit: "g" }],
        },
      ],
    },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  await ctx.dispose()
}

test.describe("Routines", () => {
  // F10-AC1
  test("are created, edited, renamed, duplicated and deleted in Settings → Meals & week", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.routine_${tag}`, "Sun", 960)
    const oats = await seedLeafFood({ name: `Oats ${tag}` })
    const name = `Breakfast ${tag}`
    const editor = page.getByTestId("routines-editor")
    const card = (text: string) =>
      editor.getByTestId("routine-card").filter({ hasText: text })

    try {
      await page.goto("/settings/meals")

      await test.step("create", async () => {
        await editor.getByRole("button", { name: "New routine" }).click()
        const sheet = page.getByTestId("routine-sheet")
        await sheet.getByLabel("Name").fill(name)
        await sheet.getByLabel("Meal").selectOption(String(slot.id))
        await sheet.getByRole("button", { name: "Add food" }).click()
        await sheet.getByLabel("Search foods").fill(oats.name)
        await sheet.getByRole("button", { name: oats.name }).click()
        const created = waitForWrite(page, /\/api\/presets$/, "POST")
        await sheet.getByRole("button", { name: "Create routine" }).click()
        await created
        await expect(sheet).toHaveCount(0)
        await expect(card(name)).toContainText(oats.name)
      })

      await test.step("edit", async () => {
        await card(name)
          .getByRole("button", { name: new RegExp(`^${name}`) })
          .click()
        const sheet = page.getByTestId("routine-sheet")
        const amount = sheet.getByLabel(`Amount of ${oats.name} in g`)
        await expect(amount).toHaveValue("100")
        await amount.fill("80")
        const saved = waitForWrite(page, /\/api\/presets\/\d+$/, "PUT")
        await amount.blur()
        const body = (await (await saved).json()) as {
          plates: { components: { amount: number }[] }[]
        }
        expect(body.plates[0]!.components[0]!.amount).toBe(80)
        await page.keyboard.press("Escape")
        await expect(sheet).toHaveCount(0)
      })

      await test.step("rename", async () => {
        await card(name)
          .getByRole("button", { name: `Actions for ${name}` })
          .click()
        await page.getByRole("menuitem", { name: "Rename" }).click()
        const dialog = page.getByRole("dialog", { name: "Rename routine" })
        await dialog.getByLabel("Name").fill(`${name} renamed`)
        const renamed = waitForWrite(page, /\/api\/presets\/\d+$/, "PUT")
        await dialog.getByRole("button", { name: "Save" }).click()
        await renamed
        await expect(card(`${name} renamed`)).toHaveCount(1)
      })

      await test.step("duplicate", async () => {
        await card(`${name} renamed`)
          .getByRole("button", { name: `Actions for ${name} renamed` })
          .click()
        await page.getByRole("menuitem", { name: "Duplicate" }).click()
        await expect(card(`${name} renamed (copy)`)).toHaveCount(1)
      })

      await test.step("delete", async () => {
        const copy = `${name} renamed (copy)`
        await card(copy)
          .getByRole("button", { name: `Actions for ${copy}` })
          .click()
        await page.getByRole("menuitem", { name: "Delete" }).click()
        const dialog = page.getByRole("dialog", { name: `Delete “${copy}”?` })
        const deleted = waitForWrite(page, /\/api\/presets\/\d+$/, "DELETE")
        await dialog.getByRole("button", { name: "Delete" }).click()
        await deleted
        await expect(card(copy)).toHaveCount(0)
        await expect(card(`${name} renamed`)).toHaveCount(1)
      })
    } finally {
      await cleanupRoutines(tag)
      await cleanupFood(oats.id)
      await cleanupSlot(slot.id)
    }
  })

  // F10-AC1, plan D2
  test("/presets is not found", async ({ page }) => {
    await page.goto("/presets")
    await expect(page.getByTestId("not-found")).toBeVisible()
  })

  // F10-AC2
  test("applying a routine from the palette creates the plates; Undo removes them and restores the replaced one", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 961)
    const other = await seedSlot(`slot.lunch_${tag}`, "Sun", 962)
    const pasta = await seedLeafFood({ name: `Pasta ${tag}` })
    const rice = await seedLeafFood({ name: `Rice ${tag}` })
    const name = `Pasta night ${tag}`
    const { start, days } = await futureWindow()
    const day = days[1]!

    try {
      const ctx = await apiRequest.newContext({ baseURL: API })
      const res = await ctx.post("/api/presets", {
        data: {
          name,
          plates: [
            {
              slot_id: slot.id,
              components: [{ food_id: pasta.id, amount: 120, unit: "g" }],
            },
            {
              slot_id: other.id,
              components: [{ food_id: pasta.id, amount: 90, unit: "g" }],
            },
          ],
        },
      })
      expect(res.ok(), await res.text()).toBeTruthy()
      await ctx.dispose()
      // The dinner cell already has Rice; lunch is empty.
      await seedPlateWithComponent(day, slot.id, {
        food_id: rice.id,
        amount: 80,
        unit: "g",
      })

      await page.goto(`/plan?start=${start}`)
      const dinner = planCell(page, day, slot.id)
      const lunch = planCell(page, day, other.id)
      await expect(dinner).toContainText(rice.name)

      const palette = await openPalette(page)
      await palette.getByRole("combobox").fill(name)
      await palette.getByRole("option", { name: new RegExp(name) }).click()
      const target = palette.getByLabel("Day")
      await target.fill(day)

      // Shift+Enter applies and replaces the existing plate.
      const applied = waitForWrite(page, /\/api\/presets\/\d+\/apply$/, "POST")
      await target.press("Shift+Enter")
      await applied
      await expect(
        page.getByText("Added 1 plate. Replaced 1 plate.")
      ).toBeVisible()
      await expect(dinner).toContainText(pasta.name)
      await expect(lunch).toContainText(pasta.name)

      const restored = waitForWrite(page, /\/api\/plan\/restore$/, "POST")
      await page.getByRole("button", { name: "Undo", exact: true }).click()
      await restored
      await expect(dinner).toContainText(rice.name)
      await expect(dinner).not.toContainText(pasta.name)
      await expect(lunch).not.toContainText(pasta.name)
    } finally {
      await cleanupRoutines(tag)
      await cleanupSlot(slot.id)
      await cleanupSlot(other.id)
      await cleanupFood(pasta.id)
      await cleanupFood(rice.id)
    }
  })

  test("the palette's Manage routines opens Settings → Meals & week", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.routine_${tag}`, "Sun", 963)
    const food = await seedLeafFood({ name: `Muesli ${tag}` })
    try {
      await seedRoutine(`Muesli ${tag}`, slot.id, food.id)
      await page.goto("/today")
      const palette = await openPalette(page)
      await palette.getByRole("option", { name: "Manage routines" }).click()
      await expect(page).toHaveURL(/\/settings\/meals$/)
      await expect(
        page.getByTestId("routine-card").filter({ hasText: `Muesli ${tag}` })
      ).toHaveCount(1)
    } finally {
      await cleanupRoutines(tag)
      await cleanupFood(food.id)
      await cleanupSlot(slot.id)
    }
  })

  test("copies a past week onto the current week", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 964)
    const food = await seedLeafFood({ name: `Quinoa ${tag}` })

    // The board copies into its window start; the dialog's source defaults
    // to the previous window. Seed its first three days.
    const ctx0 = await apiRequest.newContext({ baseURL: API })
    const planRes = await ctx0.get("/api/plan")
    const { window: win } = (await planRes.json()) as {
      window: { start: string; prev_start: string }
    }
    await ctx0.dispose()

    try {
      for (let i = 0; i < 3; i++) {
        await seedPlateWithComponent(addDays(win.prev_start, i), slot.id, {
          food_id: food.id,
          amount: 100,
          unit: "g",
        })
      }
      await page.goto("/plan")
      await page.getByRole("button", { name: "More plan actions" }).click()
      await page.getByRole("menuitem", { name: "Copy a previous week" }).click()

      const dialog = page.getByTestId("copy-from-week-dialog")
      const copied = waitForWrite(page, /\/api\/presets\/copy-week$/, "POST")
      await dialog.getByRole("button", { name: "Copy week" }).click()
      await copied

      for (let i = 0; i < 3; i++) {
        await expect(
          planCell(page, addDays(win.start, i), slot.id)
        ).toContainText(food.name)
      }
    } finally {
      await cleanupSlot(slot.id)
      await cleanupFood(food.id)
    }
  })
})
