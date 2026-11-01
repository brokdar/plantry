import { readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

import {
  API,
  addDays,
  apiRequest,
  cleanupFood,
  expect,
  futureWindow,
  openCellSheet,
  seedComposedFood,
  seedLeafFood,
  setSetting,
  test,
  uid,
} from "./helpers"

// F2-AC5: a generated week on the LLM chooser (fake provider). The template
// script plans three slots a day, so the window's cells are pinned with
// overrides: the first three slots Plan, any other slot No plate. Generate
// fills every active slot, so no parallel spec may add slots or plates
// meanwhile. That is why this spec runs alone in
// `global-settings` rather than in `ai-serial`.

const TEMPLATE = new URL(
  "./fixtures/chat-scripts/generate-week.json",
  import.meta.url
)
const NOTES = "One batch a day, cooked in the morning and eaten all day."

interface PlanView {
  slots: { id: number; sort_order: number }[]
  draft: { notes: string } | null
}

async function getPlan(start: string): Promise<PlanView> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(`/api/plan?start=${start}`)
  expect(res.ok()).toBeTruthy()
  const body = (await res.json()) as PlanView
  await ctx.dispose()
  return body
}

async function clearWindow(from: string, to: string) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.delete(`/api/plan?from=${from}&to=${to}`)
  expect(res.ok(), await res.text()).toBeTruthy()
  await ctx.dispose()
}

/** Fills the template's placeholders and slows its first turn down, so the
 *  board can watch the run while it runs. */
async function fillScript(
  recipes: number[],
  slots: number[],
  days: string[]
): Promise<string> {
  let text = await readFile(TEMPLATE, "utf8")
  recipes.forEach((id, i) => {
    text = text.replaceAll(`"{{recipe_${i + 1}}}"`, String(id))
  })
  ;["breakfast", "lunch", "dinner"].forEach((k, i) => {
    text = text.replaceAll(`"{{${k}}}"`, String(slots[i]))
  })
  days.forEach((d, i) => {
    text = text.replaceAll(`{{d${i}}}`, d)
  })
  const script = JSON.parse(text) as {
    turns: { events: { delay_ms?: number }[] }[]
  }
  script.turns[0].events[0].delay_ms = 3000
  return JSON.stringify(script)
}

test.describe("generated week on the LLM", () => {
  test("F2-AC5: progress while running, run notes, Why this?", async ({
    page,
  }) => {
    const tag = uid()
    // A window of its own, past the heuristic spec's band.
    const start = addDays((await futureWindow()).start, 7 * 20)
    const days = Array.from({ length: 7 }, (_, i) => addDays(start, i))
    const scriptPath = join(tmpdir(), `plantry-generate-week-${tag}.json`)
    const base = await seedLeafFood({
      name: `Lentils ${tag}`,
      kcal_100g: 120,
      protein_100g: 8,
      fat_100g: 1,
      carbs_100g: 20,
    })
    const foods = [base.id]
    const recipes: number[] = []
    // Generate needs the 10-recipe target (D22); the script uses 8.
    for (let i = 1; i <= 10; i++) {
      const f = await seedComposedFood({
        name: `Stew ${i} ${tag}`,
        role: "main",
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
      recipes.push(f.id)
      foods.unshift(f.id)
    }
    const ctx = await apiRequest.newContext({ baseURL: API })
    const overrides: string[] = []
    try {
      await clearWindow(start, days[6])
      const plan = await getPlan(start)
      const slots = [...plan.slots]
        .sort((a, b) => a.sort_order - b.sort_order)
        .map((s) => s.id)
      expect(slots.length).toBeGreaterThanOrEqual(3)
      for (const [i, id] of slots.entries()) {
        for (const d of days) {
          const r = await ctx.put(`/api/availability/${d}/${id}`, {
            data: { mode: i < 3 ? "plan" : "none" },
          })
          expect(r.ok(), await r.text()).toBeTruthy()
          overrides.push(`${d}/${id}`)
        }
      }
      await writeFile(scriptPath, await fillScript(recipes, slots, days))
      await setSetting("ai.fake_script", scriptPath)

      const res = await ctx.post("/api/plan/generate", {
        data: { start, chooser: "llm" },
      })
      expect(res.status(), await res.text()).toBe(202)
      const { run_id } = (await res.json()) as { run_id: number }

      // The board re-attaches to the running run and shows the bar.
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${start}`)
      const progress = page.getByTestId("generation-progress")
      await expect(progress).toBeVisible()
      await expect(progress).toBeHidden({ timeout: 20_000 })
      const run = (await (
        await ctx.get(`/api/plan/runs/${run_id}`)
      ).json()) as { status: string; repair_attempts: number }
      expect(run.status).toBe("ok")

      // The notes line carries the run's notes (from the repaired turn).
      await expect(page.getByTestId("plan-notes")).toHaveText(NOTES)

      // "Why this?" shows the plate's reason.
      const sheet = await openCellSheet(page, days[0], slots[0])
      await expect(sheet.getByText("Why this?")).toBeVisible()
      await expect(sheet.getByTestId("sheet-why")).toHaveText(
        "Cooked fresh for the day"
      )
    } finally {
      // Back to the env's script (plan-dinner.json).
      await ctx.delete("/api/settings/ai.fake_script")
      for (const o of overrides) await ctx.delete(`/api/availability/${o}`)
      await ctx.dispose()
      await rm(scriptPath, { force: true })
      await clearWindow(start, days[6])
      for (const id of foods) await cleanupFood(id)
    }
  })
})
