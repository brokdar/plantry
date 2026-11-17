import { readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

import type { Page } from "@playwright/test"
import { request as apiRequest } from "@playwright/test"

import {
  API,
  cleanupFood,
  expect,
  futureWindow,
  seedComposedFood,
  seedLeafFood,
  test,
  uid,
} from "./helpers"

// The chat agent's move op (meal editing slice 4): a scripted turn plans two
// dinners as drafts, a second one swaps them with one apply_changes move. The
// change line names both meals and Undo puts them back. Serial (ai-serial):
// the fake script is a global setting.
const TEMPLATE = new URL(
  "./fixtures/chat-scripts/assistant-move.json",
  import.meta.url
)

interface PlanView {
  slots: { id: number; name_key: string }[]
  cells: {
    date: string
    slot_id: number
    plate: { id: number; components: { food_id: number }[] } | null
  }[]
}

// Sends a message and waits for the scripted closing reply, a positive sign
// that the turn is over (an absent abort button is also true before the
// stream starts and after it failed).
async function send(page: Page, text: string, reply: string) {
  const input = page.getByTestId("assistant-composer-input")
  await input.fill(text)
  await page.getByTestId("assistant-composer-submit").click()
  await expect(
    page.getByTestId("assistant-panel").getByText(reply).first()
  ).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId("assistant-composer-abort")).toHaveCount(0)
}

test("a move onto a taken dinner swaps both meals in one change, and Undo restores them", async ({
  page,
}) => {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const tag = uid()
  const paths: string[] = []
  const leaf = await seedLeafFood({
    name: `Move beans ${tag}`,
    kcal_100g: 120,
    protein_100g: 8,
    fat_100g: 1,
    carbs_100g: 18,
  })
  const recipe = (name: string) =>
    seedComposedFood({
      name: `${name} ${tag}`,
      role: "main",
      children: [
        {
          child_id: leaf.id,
          amount: 300,
          unit: "g",
          grams: 300,
          sort_order: 0,
        },
      ],
    })
  const a = await recipe("Move curry")
  const b = await recipe("Move stew")
  const { start, days } = await futureWindow()
  const [mon, tue] = [days[2], days[3]]
  const plan = async () =>
    (await (await ctx.get(`/api/plan?start=${start}`)).json()) as PlanView
  const { slots } = await plan()
  const dinner = slots.find((s) => s.name_key === "slot.dinner")!
  const foodAt = async (date: string) =>
    (await plan()).cells.find((c) => c.date === date && c.slot_id === dinner.id)
      ?.plate?.components[0]?.food_id

  async function useScript(plateA: number) {
    let script = await readFile(TEMPLATE, "utf8")
    for (const [key, value] of Object.entries({
      a: a.id,
      b: b.id,
      dinner: dinner.id,
      plate_a: plateA,
    })) {
      script = script.replaceAll(`"{{${key}}}"`, String(value))
    }
    script = script.replaceAll("{{mon}}", mon).replaceAll("{{tue}}", tue)
    const path = join(
      tmpdir(),
      `plantry-assistant-move-${tag}-${paths.length}.json`
    )
    paths.push(path)
    await writeFile(path, script)
    const res = await ctx.put("/api/settings/ai.fake_script", {
      data: { value: path },
    })
    expect(res.ok(), await res.text()).toBeTruthy()
  }

  try {
    for (const d of [mon, tue]) {
      const res = await ctx.put(`/api/availability/${d}/${dinner.id}`, {
        data: { mode: "plan" },
      })
      expect(res.ok(), await res.text()).toBeTruthy()
    }
    await useScript(0)
    await page.setViewportSize({ width: 1440, height: 900 })
    // Open the panel from the toolbar, not with `?assistant=1`: a page that
    // loads with the panel open restores the latest conversation of the shared
    // DB (another spec's, or an earlier run's), whose change lines would count
    // as ours and whose switch can land mid-turn.
    await page.goto(`/plan?start=${start}`)
    await page.getByRole("button", { name: "Ask assistant" }).click()
    const panel = page.getByTestId("assistant-panel")
    await expect(panel).toBeVisible()

    await send(page, "plan two dinners", "Both dinners are planned.")
    await expect(panel.getByTestId("assistant-change")).toHaveCount(1)
    expect(await foodAt(mon)).toBe(a.id)
    expect(await foodAt(tue)).toBe(b.id)
    const plateA = (await plan()).cells.find(
      (c) => c.date === mon && c.slot_id === dinner.id
    )!.plate!.id

    await useScript(plateA)
    await send(page, "swap them", "Swapped the two dinners.")
    const line = panel
      .getByTestId("assistant-change")
      .filter({ hasText: "swapped" })
    await expect(line).toHaveCount(1)
    await expect(line).toContainText(a.name)
    await expect(line).toContainText(b.name)
    expect(await foodAt(mon)).toBe(b.id)
    expect(await foodAt(tue)).toBe(a.id)

    await line.getByTestId("assistant-change-undo").click()
    await expect(line).toHaveAttribute("data-state", "undone")
    await expect.poll(() => foodAt(mon)).toBe(a.id)
    expect(await foodAt(tue)).toBe(b.id)
  } finally {
    await ctx.delete("/api/settings/ai.fake_script")
    for (const c of (await plan()).cells) {
      if (c.plate && [mon, tue].includes(c.date) && c.slot_id === dinner.id) {
        await ctx.delete(`/api/plates/${c.plate.id}`)
      }
    }
    for (const d of [mon, tue]) {
      await ctx.delete(`/api/availability/${d}/${dinner.id}`)
    }
    await cleanupFood(a.id)
    await cleanupFood(b.id)
    await cleanupFood(leaf.id)
    for (const p of paths) await rm(p, { force: true })
    await ctx.dispose()
  }
})
