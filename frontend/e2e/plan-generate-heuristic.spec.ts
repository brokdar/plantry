import type { APIResponse, Locator, Page, Response } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  mockSettingValues,
  openCellSheet,
  planCell,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// Generate fills every active slot of the window, so plates land in slots
// other parallel specs own and later delete, and Accept/Unaccept cover the
// whole window. That is why this spec runs alone in `global-settings`.

const padlock = (cell: Locator) => cell.getByTestId("cell-padlock")

function waitForPost(page: Page, path: string) {
  return page.waitForResponse(
    (r) => r.url().includes(path) && r.request().method() === "POST"
  )
}
// F2-AC4: a generated week on the heuristic chooser. The shared library holds
// other specs' recipes, so the assertions follow plate ids and states rather
// than names: each regenerate replaces its target with a new draft.

interface PlanPlate {
  id: number
  date: string
  slot_id: number
  state: "draft" | "confirmed"
  components: { id: number; food_id: number }[]
}

async function slotPlates(start: string, slot: number): Promise<PlanPlate[]> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(`/api/plan?start=${start}`)
  expect(res.ok()).toBeTruthy()
  const { cells } = (await res.json()) as {
    cells: { slot_id: number; plate: PlanPlate | null }[]
  }
  await ctx.dispose()
  return cells.flatMap((c) => (c.slot_id === slot && c.plate ? [c.plate] : []))
}

async function clearWindow(from: string, to: string) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.delete(`/api/plan?from=${from}&to=${to}`)
  expect(res.ok(), await res.text()).toBeTruthy()
  await ctx.dispose()
}

/** Waits for the run the POST answered to finish ok. */
async function runFinished(post: Promise<APIResponse | Response>) {
  const res = await post
  expect(res.status(), await res.text()).toBe(202)
  const { run_id } = (await res.json()) as { run_id: number }
  const ctx = await apiRequest.newContext({ baseURL: API })
  await expect
    .poll(async () => {
      const r = await ctx.get(`/api/plan/runs/${run_id}`)
      return ((await r.json()) as { status: string }).status
    })
    .toBe("ok")
  await ctx.dispose()
}

test.describe("generated week on the heuristic", () => {
  test("F2-AC4: generate, Replace, Different side, Regenerate day, Accept and shop, Undo", async ({
    page,
  }) => {
    const tag = uid()
    const { start, days } = await futureWindow()
    const slot = await seedSlot(`slot.prg-${tag}`, "Moon", 99981)
    const base = await seedLeafFood({
      name: `Beans ${tag}`,
      kcal_100g: 120,
      protein_100g: 8,
      fat_100g: 1,
      carbs_100g: 20,
    })
    const foods = [base.id]
    // Generate needs the 10-recipe target (D22) even on a fresh DB.
    const recipes = [
      ...["Chili", "Curry", "Dal", "Ragout", "Hash", "Pilaf"].map(
        (n) => [n, "main"] as const
      ),
      ...["Rice", "Bread"].map((n) => [n, "side_starch"] as const),
      ...["Greens", "Slaw"].map((n) => [n, "side_veg"] as const),
    ]
    for (const [dish, role] of recipes) {
      const f = await seedComposedFood({
        name: `${dish} ${tag}`,
        role,
        reference_portions: 2,
        keep_days: 2,
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
      foods.unshift(f.id)
    }
    try {
      await clearWindow(start, days[6])
      const ctx = await apiRequest.newContext({ baseURL: API })
      await runFinished(
        ctx.post("/api/plan/generate", {
          data: { start, chooser: "heuristic" },
        })
      )
      await ctx.dispose()
      const generated = await slotPlates(start, slot.id)
      expect(generated.length).toBeGreaterThan(0)
      expect(generated.every((p) => p.state === "draft")).toBe(true)

      // Heuristic-only: the board sees no assistant.
      await mockSettingValues(page, { "ai.provider": "" })
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${start}`)
      await expect(page.getByTestId("plan-summary")).toHaveText(/^Draft · /)
      for (const p of generated) {
        const cell = planCell(page, p.date, slot.id)
        await expect(cell).not.toHaveAttribute("data-cell-empty", "")
        await expect(padlock(cell)).toHaveCount(0)
      }

      // Replace: the plate gives way to a new draft in its cell.
      const target = generated[0]
      let sheet = await openCellSheet(page, target.date, slot.id)
      let post = waitForPost(page, "/api/plan/regenerate")
      await sheet.getByRole("button", { name: "Replace" }).click()
      await runFinished(post)
      await expect
        .poll(async () => (await slotPlates(start, slot.id)).map((p) => p.id))
        .not.toContain(target.id)

      // Different side, on a plate that has one.
      let plates = await slotPlates(start, slot.id)
      const withSide = plates.find((p) => p.components.length > 1)
      expect(withSide, "a generated plate with a side").toBeTruthy()
      const sideIds = withSide!.components.map((c) => c.id)
      sheet = await openCellSheet(page, withSide!.date, slot.id)
      post = waitForPost(page, "/api/plan/regenerate")
      await sheet.getByRole("button", { name: "Different side" }).click()
      await runFinished(post)
      plates = await slotPlates(start, slot.id)
      const afterSide = plates
        .filter((p) => p.date === withSide!.date)
        .flatMap((p) => p.components.map((c) => c.id))
      expect(afterSide.length).toBeGreaterThan(0)
      expect(afterSide).not.toEqual(sideIds)

      // Regenerate day: every draft plate of that day is replaced.
      const day = plates[plates.length - 1].date
      const dayIds = plates.filter((p) => p.date === day).map((p) => p.id)
      sheet = await openCellSheet(page, day, slot.id)
      post = waitForPost(page, "/api/plan/regenerate")
      await sheet.getByRole("button", { name: "Regenerate day" }).click()
      await runFinished(post)
      plates = await slotPlates(start, slot.id)
      const newDay = plates.filter((p) => p.date === day)
      expect(newDay.length).toBeGreaterThan(0)
      for (const p of newDay) expect(dayIds).not.toContain(p.id)
      await expect(planCell(page, day, slot.id)).not.toHaveAttribute(
        "data-cell-empty",
        ""
      )

      // Accept and shop confirms the drafts; Undo returns them to draft.
      await page.goto(`/plan?start=${start}`)
      const accepted = waitForPost(page, "/api/plan/accept")
      await page.getByRole("button", { name: "Accept and shop" }).click()
      expect((await accepted).ok()).toBeTruthy()
      await expect(page).toHaveURL(`/shop?from=${start}&to=${days[6]}`)
      const unaccepted = waitForPost(page, "/api/plan/unaccept")
      await page.getByRole("button", { name: "Undo" }).click()
      expect((await unaccepted).ok()).toBeTruthy()
      expect(
        (await slotPlates(start, slot.id)).every((p) => p.state === "draft")
      ).toBe(true)
      await page.goto(`/plan?start=${start}`)
      for (const p of plates) {
        await expect(padlock(planCell(page, p.date, slot.id))).toHaveCount(0)
      }

      // Accepting for good leaves padlocks on /plan.
      await page.getByRole("button", { name: "Accept and shop" }).click()
      await expect(page).toHaveURL(/\/shop\?/)
      await page.goto(`/plan?start=${start}`)
      for (const p of plates) {
        await expect(padlock(planCell(page, p.date, slot.id))).toHaveCount(1)
      }
    } finally {
      // Generate filled every slot of the window, not just ours.
      await clearWindow(start, days[6])
      await cleanupSlot(slot.id)
      for (const id of foods) await cleanupFood(id)
    }
  })
})
