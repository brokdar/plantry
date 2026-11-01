import { readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

import type { APIRequestContext, Page } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  expect,
  futureWindow,
  planCell,
  seedComposedFood,
  seedLeafFood,
  seedRecipeLibrary,
  test,
  uid,
} from "./helpers"

// Phase 4 acceptance AC-1 to AC-4.1 (F3): the assistant changes a draft
// window through scripted chat turns (fake provider, assistant-drafts.json).
// Global-settings: it turns freezing on, and accepting the window confirms
// every draft in it.
//
// Fixture: "Batch cooker with freezer" in a window of its own. Monday dinner
// cooks a freezable Chili (4 portions, session of its own) with a Rice side;
// Tuesday dinner has a main with a Bulgur side. The plates come from the
// plates API (unlocked to drafts), not a heuristic run, so the Chili and the
// side are exactly what the scripted turns name.

const TEMPLATE = new URL(
  "./fixtures/chat-scripts/assistant-drafts.json",
  import.meta.url
)

interface PlanPlate {
  id: number
  state: "draft" | "confirmed"
  components: { id: number; food_id: number; cook_session_id: number | null }[]
}
interface PlanView {
  slots: { id: number; name_key: string }[]
  cells: { date: string; slot_id: number; plate: PlanPlate | null }[]
  sessions: {
    id: number
    food_id: number
    portions: number
    frozen_portions: number
    state: string
  }[]
  issues: unknown[]
}
interface Run {
  id: number
  kind: string
  status: string
  prompt_version: string | null
  outcome: string | null
  edits: number | null
}
interface RunDetail extends Run {
  scope: { kind?: string } | null
  chooser: string | null
  instruction: string | null
  prompt_text: string | null
  steps: { name: string; result: string | null; is_error: boolean }[]
}

async function getJSON<T>(ctx: APIRequestContext, path: string): Promise<T> {
  const res = await ctx.get(path)
  expect(res.ok(), `${path}: ${await res.text()}`).toBeTruthy()
  return (await res.json()) as T
}

async function createPlate(
  ctx: APIRequestContext,
  date: string,
  slot: number,
  foods: number[]
): Promise<{ id: number; components: PlanPlate["components"] }> {
  const res = await ctx.post("/api/plates", {
    data: {
      date,
      slot_id: slot,
      components: foods.map((food_id) => ({ food_id, portions: 1 })),
    },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  const plate = (await res.json()) as PlanPlate
  if (plate.state !== "draft") {
    const unlock = await ctx.post(`/api/plates/${plate.id}/unlock`)
    expect(unlock.ok(), await unlock.text()).toBeTruthy()
  }
  return plate
}

/** Sends a message in the open panel and waits for the turn to finish. */
async function send(page: Page, text: string) {
  const input = page.getByTestId("assistant-composer-input")
  await input.fill(text)
  await page.getByTestId("assistant-composer-submit").click()
  await expect(page.getByTestId("assistant-composer-abort")).toHaveCount(0, {
    timeout: 60_000,
  })
}

const weekday = (iso: string) =>
  new Date(`${iso}T12:00:00`).toLocaleDateString("en", { weekday: "long" })

test("AC-1 to AC-4.1: portions and freezing, a different side, the unlock card, accepted runs", async ({
  page,
}) => {
  test.setTimeout(180_000)
  const tag = uid()
  const { start, days } = await futureWindow()
  const monday = days.find((d) => new Date(`${d}T12:00:00`).getDay() === 1)!
  const tuesday = days.find((d) => new Date(`${d}T12:00:00`).getDay() === 2)!
  const ctx = await apiRequest.newContext({ baseURL: API })
  const scriptPaths: string[] = []

  const settings = await getJSON<{ items: { key: string; value: string }[] }>(
    ctx,
    "/api/settings"
  )
  const freezingBefore = settings.items.find(
    (i) => i.key === "plan.freezing_enabled"
  )?.value

  const kidney = await seedLeafFood({
    name: `Kidney ${tag}`,
    kcal_100g: 100,
    protein_100g: 8,
    fat_100g: 1,
    carbs_100g: 15,
  })
  // The other recipes use a leaf of their own, so the Chili's scales alone.
  const plain = await seedLeafFood({
    name: `Plain ${tag}`,
    kcal_100g: 100,
    protein_100g: 8,
    fat_100g: 1,
    carbs_100g: 15,
  })
  const main = (name: string, role: string, freezable = false) =>
    seedComposedFood({
      name: `${name} ${tag}`,
      role,
      reference_portions: 4,
      keep_days: 3,
      freezable,
      children: [
        {
          child_id: (name === "Chili" ? kidney : plain).id,
          amount: 400,
          unit: "g",
          grams: 400,
          sort_order: 0,
        },
      ],
    })
  // The generator needs its 10-recipe target (D22) when the side is re-chosen.
  const library = await seedRecipeLibrary(tag)
  const chili = await main("Chili", "main", true)
  const stew = await main("Stew", "main")
  const rice = await main("Rice", "side_starch")
  const bulgur = await main("Bulgur", "side_starch")
  const roast = await main("Roast potatoes", "side_starch")
  const foods = [chili, stew, rice, bulgur, roast].map((f) => f.id)

  try {
    const put = await ctx.put("/api/settings/plan.freezing_enabled", {
      data: { value: "true" },
    })
    expect(put.ok(), await put.text()).toBeTruthy()
    const clear = await ctx.delete(`/api/plan?from=${start}&to=${days[6]}`)
    expect(clear.ok(), await clear.text()).toBeTruthy()

    const { slots } = await getJSON<PlanView>(ctx, `/api/plan?start=${start}`)
    const dinner = slots.find((s) => s.name_key === "slot.dinner")!
    expect(dinner, "the Dinner slot").toBeTruthy()

    const monPlate = await createPlate(ctx, monday, dinner.id, [
      chili.id,
      rice.id,
    ])
    const tuePlate = await createPlate(ctx, tuesday, dinner.id, [
      stew.id,
      bulgur.id,
    ])
    const chiliSession = monPlate.components[0].cook_session_id!
    const monSide = monPlate.components[1].id
    const tueSide = tuePlate.components[1].id
    const resize = await ctx.put(`/api/cook-sessions/${chiliSession}`, {
      data: { portions: 4, frozen_portions: 0, note: null },
    })
    expect(resize.ok(), await resize.text()).toBeTruthy()

    // The fake client is cached per path: a refill gets a path of its own.
    async function useScript(over: Record<string, number> = {}) {
      let script = await readFile(TEMPLATE, "utf8")
      for (const [key, value] of Object.entries({
        chili_session: chiliSession,
        tue_side_component: tueSide,
        mon_plate: monPlate.id,
        mon_side_component: monSide,
        roast: roast.id,
        dinner: dinner.id,
        tue_plate: tuePlate.id,
        ...over,
      })) {
        script = script.replaceAll(`"{{${key}}}"`, String(value))
      }
      script = script.replaceAll("{{tue}}", tuesday)
      const path = join(
        tmpdir(),
        `plantry-assistant-drafts-${tag}-${scriptPaths.length}.json`
      )
      scriptPaths.push(path)
      await writeFile(path, script)
      const res = await ctx.put("/api/settings/ai.fake_script", {
        data: { value: path },
      })
      expect(res.ok(), await res.text()).toBeTruthy()
    }
    await useScript()

    const plan = () => getJSON<PlanView>(ctx, `/api/plan?start=${start}`)
    const session = async () =>
      (await plan()).sessions.find((s) => s.id === chiliSession)!
    const beans = async () => {
      const list = await getJSON<{
        sections: { items: { food_id: number; total_grams: number }[] }[]
      }>(
        ctx,
        `/api/shopping-list?from=${start}&to=${days[6]}&include_drafts=true`
      )
      return list.sections
        .flatMap((s) => s.items)
        .find((i) => i.food_id === kidney.id)!.total_grams
    }
    const maxRunId = async () =>
      Math.max(
        0,
        ...(
          await getJSON<{ items: Run[] }>(ctx, "/api/ai/runs?limit=200")
        ).items.map((r) => r.id)
      )
    const runsAfter = async (id: number, kind: string) =>
      (
        await getJSON<{ items: Run[] }>(
          ctx,
          `/api/ai/runs?kind=${kind}&limit=200`
        )
      ).items
        .filter((r) => r.id > id)
        .sort((a, b) => a.id - b.id)

    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/plan?start=${start}`)
    await expect(page.getByRole("grid")).toBeVisible()

    // ── AC-1: Make the chili 6 portions and freeze 2 ──────────────────────
    await page.getByRole("heading", { level: 1 }).click()
    await page.keyboard.press("a")
    const panel = page.getByRole("complementary", { name: "Assistant" })
    await expect(panel).toBeVisible()
    await expect(page.getByRole("dialog")).toHaveCount(0)
    expect((await panel.boundingBox())!.width).toBe(380)
    await expect(panel).toContainText(
      "Changes go into your draft. Confirmed plates are kept unless you agree."
    )
    await expect(panel.getByRole("button", { name: /freeze 2/ })).toBeVisible()
    // AC-1.3: the Chili's cook badge reads 4 now, 6 after the turn.
    const chiliBadge = (portions: number) =>
      page
        .locator("[data-batch-badge]")
        .filter({ hasText: `× ${portions}` })
        .first()
    await expect(chiliBadge(4)).toBeVisible()

    // Layout (DoD blocker): below 1280 px the panel floats over the board
    // instead of squeezing it, so the cells keep their width and the review
    // action stays on the page. Back to 1440 px for the rest.
    const monCell = planCell(page, monday, dinner.id)
    for (const [width, minCell] of [
      [1180, 95],
      [820, 55],
    ] as const) {
      await page.setViewportSize({ width, height: 900 })
      await expect(panel).toBeVisible()
      await expect(page.getByRole("dialog")).toHaveCount(0)
      const box = (await panel.boundingBox())!
      expect(box.width).toBe(380)
      expect(box.x + box.width).toBe(width)
      expect((await monCell.boundingBox())!.width).toBeGreaterThan(minCell)
      await expect(
        page.getByRole("button", { name: "Accept and shop" }).first()
      ).toBeVisible()
    }
    await page.setViewportSize({ width: 1440, height: 900 })
    await expect(panel).toBeVisible()
    expect((await panel.boundingBox())!.width).toBe(380)

    const beansBefore = await beans()
    expect(beansBefore).toBe(400)
    const issuesBefore = (await plan()).issues.length
    const runMark = await maxRunId()
    await send(page, "Make the chili 6 portions and freeze 2")

    const sentence = `${chili.name}: 6 portions, 2 frozen`
    const line = panel.getByTestId("assistant-change")
    await expect(line).toHaveCount(1)
    await expect(line).toContainText(sentence)
    await expect(page.getByTestId("assistant-live")).toHaveText(sentence)
    await expect(panel.getByRole("button", { name: /^Undo/ })).toBeVisible()
    await expect(panel).not.toContainText(
      /get_plan|set_session_portions|regenerate|submit_plan|tool_use/
    )
    await expect(planCell(page, monday, dinner.id)).toHaveAttribute(
      "data-pulse",
      ""
    )
    await expect(
      panel.getByTestId("assistant-message-assistant").last()
    ).toContainText("6 portions")

    await expect(chiliBadge(6)).toBeVisible() // no reload
    const changed = await session()
    expect(changed).toMatchObject({
      portions: 6,
      frozen_portions: 2,
      state: "draft",
    })
    expect(await beans()).toBeCloseTo(beansBefore * 1.5, 5)
    expect((await plan()).issues.length).toBe(issuesBefore)

    const [chat1] = await runsAfter(runMark, "chat")
    const detail1 = await getJSON<RunDetail>(ctx, `/api/ai/runs/${chat1.id}`)
    expect(detail1).toMatchObject({
      kind: "chat",
      status: "ok",
      prompt_version: "v5",
    })
    expect(detail1.instruction).toBe("Make the chili 6 portions and freeze 2")
    expect(detail1.steps.map((s) => s.name)).toEqual(
      expect.arrayContaining(["get_plan", "set_session_portions"])
    )
    expect(detail1.prompt_text).toContain(start)
    expect(detail1.prompt_text).toContain(chili.name)
    // AC-1.6: today's date, the window's dates with weekdays, the stored
    // conversation and window.
    expect(detail1.prompt_text).toMatch(
      /Today is [A-Z][a-z]+ \d{4}-\d{2}-\d{2}\./
    )
    expect(detail1.prompt_text).toContain(`${weekday(monday)} ${monday}`)
    expect(detail1.prompt_text).toContain(`${weekday(tuesday)} ${tuesday}`)
    expect(detail1.prompt_text).toContain(days[6])
    // The run belongs to the conversation: its change is listed there.
    const conversations = await getJSON<{ items: { id: number }[] }>(
      ctx,
      "/api/ai/conversations?limit=50"
    )
    let found = false
    for (const c of conversations.items) {
      const detail = await getJSON<{ changes: { run_id: number }[] }>(
        ctx,
        `/api/ai/conversations/${c.id}`
      )
      if (detail.changes.some((ch) => ch.run_id === chat1.id)) found = true
    }
    expect(found, "the chat run's conversation").toBe(true)

    // Undo restores 4 / 0 and marks the line.
    await panel.getByRole("button", { name: /^Undo/ }).click()
    await expect(line).toContainText("Undone")
    await expect(panel.getByTestId("assistant-change-undo")).toHaveCount(0)
    expect(await session()).toMatchObject({ portions: 4, frozen_portions: 0 })

    // Once more, to keep it for the accepted runs below.
    await send(page, "Make the chili 6 portions and freeze 2")
    await expect(
      panel.getByTestId("assistant-change").filter({ hasText: sentence })
    ).toHaveCount(2)
    expect(await session()).toMatchObject({ portions: 6, frozen_portions: 2 })

    // ── AC-2: Different side on Tuesday ───────────────────────────────────
    const before = await plan()
    const sideRunMark = await maxRunId()
    await send(page, "different side on Tuesday")
    const sideLine = panel
      .getByTestId("assistant-change")
      .filter({ hasText: `${weekday(tuesday)} dinner: new side` })
    await expect(sideLine).toHaveCount(1)
    await expect(sideLine).toHaveText(
      new RegExp(`${weekday(tuesday)} dinner: new side ${roast.name}`)
    )

    const after = await plan()
    const tue = (p: PlanView) =>
      p.cells.find((c) => c.date === tuesday && c.slot_id === dinner.id)!.plate!
    expect(tue(after).components.map((c) => c.food_id)).toEqual([
      stew.id,
      roast.id,
    ])
    expect(tue(after).components[0]).toEqual(tue(before).components[0])
    const others = (p: PlanView) =>
      p.cells.filter((c) => !(c.date === tuesday && c.slot_id === dinner.id))
    expect(others(after)).toEqual(others(before))
    const sideSessions = (p: PlanView) =>
      p.sessions.filter(
        (s) => s.food_id !== bulgur.id && s.food_id !== roast.id
      )
    expect(sideSessions(after)).toEqual(sideSessions(before))

    const [chat2] = await runsAfter(sideRunMark, "chat")
    const regen = (await runsAfter(sideRunMark, "regenerate"))[0]
    const regenDetail = await getJSON<RunDetail>(
      ctx,
      `/api/ai/runs/${regen.id}`
    )
    expect(regenDetail).toMatchObject({
      kind: "regenerate",
      status: "ok",
      chooser: "llm",
    })
    expect(regenDetail.scope?.kind).toBe("side")
    const chat2Detail = await getJSON<RunDetail>(
      ctx,
      `/api/ai/runs/${chat2.id}`
    )
    const regenStep = chat2Detail.steps.find((s) => s.name === "regenerate")!
    expect(regenStep.is_error).toBe(false)
    expect(regenStep.result).toContain(`"run_id":${regen.id}`)

    // Undo brings the old side back; the new request swaps it again.
    await sideLine.getByRole("button", { name: /^Undo/ }).click()
    await expect(sideLine).toContainText("Undone")
    expect(tue(await plan()).components.map((c) => c.food_id)).toEqual([
      stew.id,
      bulgur.id,
    ])
    // Undo restores the side as a new component: point the script at it.
    await useScript({
      tue_side_component: tue(await plan()).components[1].id,
    })
    const regenMark = await maxRunId()
    await send(page, "different side on Tuesday")
    expect(tue(await plan()).components.map((c) => c.food_id)).toEqual([
      stew.id,
      roast.id,
    ])

    // ── Accept and shop (AC-3 step 1 precondition, AC-4.1) ────────────────
    await page.getByRole("button", { name: "Accept and shop" }).click()
    await expect(page).toHaveURL(/\/shop/)
    await expect
      .poll(async () =>
        (await plan()).cells
          .filter((c) => c.plate)
          .every((c) => c.plate!.state === "confirmed")
      )
      .toBe(true)

    const chatRuns = await runsAfter(runMark, "chat")
    expect(chatRuns).toHaveLength(4)
    // Both chat runs that stayed in the plan and the regenerate run are
    // accepted: the side request's chat run only started the regenerate run
    // (which wrote under the generator), and follows its outcome.
    const finalChili = chatRuns[1]
    const finalSide = chatRuns[3]
    const finalRegen = (await runsAfter(regenMark, "regenerate"))[0]
    await expect
      .poll(async () => {
        const runs = await runsAfter(runMark, "chat")
        const re = await runsAfter(regenMark, "regenerate")
        return [
          runs.find((r) => r.id === finalChili.id)?.outcome,
          runs.find((r) => r.id === finalSide.id)?.outcome,
          re.find((r) => r.id === finalRegen.id)?.outcome,
        ]
      })
      .toEqual(["accepted", "accepted", "accepted"])

    // ── AC-3: confirmed rows are protected ────────────────────────────────
    await page.goto(`/plan?start=${start}&assistant=1`)
    const panel3 = page.getByRole("complementary", { name: "Assistant" })
    // The reload reopens the latest conversation with its change lines;
    // AC-3 starts a conversation of its own.
    await expect(panel3.getByTestId("assistant-change").first()).toBeVisible()
    await panel3.getByTestId("assistant-new-conversation").click()
    // Undoing the Chili change put the plate back as new component rows.
    const monNow = (await plan()).cells.find(
      (c) => c.date === monday && c.slot_id === dinner.id
    )!.plate!
    await useScript({
      mon_plate: monNow.id,
      mon_side_component: monNow.components[1].id,
    })
    const mark3 = await maxRunId()
    await send(page, "remove the side on Monday")
    const card = panel3.getByTestId("assistant-unlock-card")
    await expect(card).toBeVisible()
    await expect(card).toContainText("Monday dinner")
    await expect(card).toContainText(chili.name)
    await expect(card).toContainText(rice.name)
    // The card is the thing to act on: it is scrolled into view.
    await expect(card).toBeInViewport()
    await expect(
      panel3.getByTestId("assistant-message-assistant").last()
    ).toContainText("say yes")
    await expect(panel3.getByTestId("assistant-change")).toHaveCount(0)
    await expect(panel3).not.toContainText(
      /remove_plate_component|unlock\b.*\{/
    )
    // The removal failed on the confirmed row; nothing changed.
    const [chat3] = await runsAfter(mark3, "chat")
    const failed = (
      await getJSON<RunDetail>(ctx, `/api/ai/runs/${chat3.id}`)
    ).steps.find((x) => x.name === "remove_plate_component")!
    expect(failed.is_error).toBe(true)
    expect(failed.result).toContain("confirmed")
    // The stored conversation keeps the failed result's message key.
    const latest = (
      await getJSON<{ items: { id: number }[] }>(
        ctx,
        "/api/ai/conversations?limit=1"
      )
    ).items[0]
    const latestDetail = await getJSON<{ messages: unknown[] }>(
      ctx,
      `/api/ai/conversations/${latest.id}`
    )
    expect(JSON.stringify(latestDetail.messages)).toContain(
      "error.plan.row_confirmed"
    )
    let mon = (await plan()).cells.find(
      (c) => c.date === monday && c.slot_id === dinner.id
    )!.plate!
    expect(mon.state).toBe("confirmed")
    expect(mon.components).toHaveLength(2)

    // A reload keeps the question: the card is rebuilt from the stored
    // preview and the conversation is the one that was open.
    await page.reload()
    const card2 = panel3.getByTestId("assistant-unlock-card")
    await expect(card2).toBeVisible()
    await expect(card2).toContainText(chili.name)
    await expect(card2).toBeInViewport()
    const yes = card2.getByRole("button", { name: "Yes, unlock" })
    expect((await yes.boundingBox())!.height).toBeGreaterThanOrEqual(36)

    await yes.click()
    await expect(
      panel3.getByTestId("assistant-message-user").last()
    ).toHaveText("Yes, unlock")
    await expect(
      panel3.getByTestId("assistant-change").filter({ hasText: "unlocked" })
    ).toHaveCount(1, { timeout: 60_000 })
    await expect(
      panel3
        .getByTestId("assistant-change")
        .filter({ hasText: `${weekday(monday)}: ${rice.name} removed` })
    ).toHaveCount(1)
    await expect(panel3.getByTestId("assistant-unlock-card")).toHaveCount(0)
    // The transcript followed the new lines to the bottom.
    await expect(panel3.getByTestId("assistant-change").last()).toBeInViewport()
    mon = (await plan()).cells.find(
      (c) => c.date === monday && c.slot_id === dinner.id
    )!.plate!
    expect(mon.state).toBe("draft")
    expect(mon.components.map((c) => c.food_id)).toEqual([chili.id])
    // AC-3.2 (unlock events with actor agent) is proven by the backend test
    // TestTool_UnlockNeedsTheUsersYesInALaterTurn (no API lists plate events).
  } finally {
    for (const path of scriptPaths) await rm(path, { force: true })
    await ctx.delete("/api/settings/ai.fake_script")
    if (freezingBefore !== undefined) {
      await ctx.put("/api/settings/plan.freezing_enabled", {
        data: { value: freezingBefore },
      })
    }
    await ctx.delete(`/api/plan?from=${start}&to=${days[6]}`)
    await ctx.dispose()
    for (const id of foods) await cleanupFood(id)
    await library.cleanup()
    await cleanupFood(kidney.id)
    await cleanupFood(plain.id)
  }
})

// Phase 4 AC-5: the cell pulse is decoration and is off under reduced motion.
test("AC-5: the change pulse is off under reduced motion", async ({ page }) => {
  await page.goto("/plan")
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible()
  const pulseName = () =>
    page.evaluate(() => {
      const el = document.createElement("div")
      el.setAttribute("data-pulse", "")
      document.body.append(el)
      const name = getComputedStyle(el).animationName
      el.remove()
      return name
    })
  await page.emulateMedia({ reducedMotion: "no-preference" })
  expect(await pulseName()).toBe("cell-pulse")
  await page.emulateMedia({ reducedMotion: "reduce" })
  expect(await pulseName()).toBe("none")
})
