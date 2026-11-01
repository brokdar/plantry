import type { APIRequestContext, Page } from "@playwright/test"

import {
  API,
  addDays,
  apiRequest,
  cleanupSlot,
  expect,
  mockSettingValues,
  planCell,
  seedRecipeLibrary,
  seedSlot,
  test,
  uid,
} from "./helpers"

// F4-AC1 = AC-5 (UX §5.10). The browser's clock is set to a Wednesday-like
// day 3 of a window 88 weeks ahead, so "today" is mid-week there: the board
// treats that window as the current one and sends `today` with every plan
// read and the re-plan. A Chili batch cooks today and feeds the next two
// days; marking those days No plate frees its portions. Re-plan the rest of
// the week runs a real heuristic generation over every empty cell from
// today on, so this spec runs alone in `global-settings`.

interface PlanPlate {
  id: number
  date: string
  slot_id: number
  state: "draft" | "confirmed"
}

interface Session {
  portions: number
  frozen_portions: number
}

const weekday = (iso: string) =>
  new Date(`${iso}T12:00:00`).toLocaleDateString("en-US", { weekday: "long" })

async function windowPlates(
  ctx: APIRequestContext,
  start: string,
  today: string
) {
  const res = await ctx.get(`/api/plan?start=${start}&today=${today}`)
  expect(res.ok()).toBeTruthy()
  const { cells } = (await res.json()) as {
    cells: {
      date: string
      slot_id: number
      mode: string
      plate: PlanPlate | null
    }[]
  }
  return cells
}

async function session(ctx: APIRequestContext, id: number): Promise<Session> {
  const res = await ctx.get(`/api/cook-sessions/${id}`)
  expect(res.ok()).toBeTruthy()
  return (await res.json()) as Session
}

/** Opens the overflow's Re-plan from today and ticks Thu and Fri whole. */
async function openAndMark(page: Page, days: string[]) {
  await page.getByRole("button", { name: "More plan actions" }).click()
  await page.getByRole("menuitem", { name: "Re-plan from today" }).click()
  const sheet = page.getByTestId("replan-sheet")
  await expect(
    sheet.getByRole("heading", { name: "Re-plan from today" })
  ).toBeVisible()
  for (const d of [days[4], days[5]]) {
    const day = sheet.getByRole("group", {
      name: new RegExp(`^${weekday(d)}`),
    })
    // The chip is the label; its checkbox is visually hidden.
    await day.getByText("Whole day").click()
    await expect(day.getByLabel("Whole day")).toBeChecked()
  }
  return sheet
}

test("F4-AC1: re-plan from today: Keep them changes nothing; Change them removes, freezes and fills the week", async ({
  page,
}) => {
  // Seeding ten recipes and a real generation take longer than the default.
  test.setTimeout(120_000)
  const tag = uid()
  const ctx = await apiRequest.newContext({ baseURL: API })
  const anchored = await ctx.get("/api/plan")
  const { window } = (await anchored.json()) as { window: { start: string } }
  const start = addDays(window.start, 7 * 88)
  const days = Array.from({ length: 7 }, (_, i) => addDays(start, i))
  const today = days[3]
  const end = days[6]
  let slotId: number | undefined
  let library: Awaited<ReturnType<typeof seedRecipeLibrary>> | undefined
  let slotIds: number[] = []

  // plan.freezing_enabled as it was, to put back afterwards.
  const settings = (await (await ctx.get("/api/settings")).json()) as {
    items: { key: string; value: string; source: string }[]
  }
  const freezingBefore = settings.items.find(
    (i) => i.key === "plan.freezing_enabled"
  )
  try {
    const freezing = await ctx.put("/api/settings/plan.freezing_enabled", {
      data: { value: "true" },
    })
    expect(freezing.ok(), await freezing.text()).toBeTruthy()
    // The day's first slot, so today's other meals can eat the Chili cooked
    // here.
    const slot = await seedSlot(`slot.rpl-${tag}`, "Moon", -1)
    slotId = slot.id
    // The generator needs the 10-recipe target (D22) even on a fresh DB.
    library = await seedRecipeLibrary(tag)
    const [chili, curry] = library.mains

    const clear = await ctx.delete(`/api/plan?from=${start}&to=${end}`)
    expect(clear.ok(), await clear.text()).toBeTruthy()

    // A past plate (day 1) and the Chili batch: cooked today, eaten Thu, Fri.
    const past = await ctx.post("/api/plates", {
      data: {
        date: days[1],
        slot_id: slot.id,
        components: [{ food_id: curry.id }],
      },
    })
    expect(past.ok(), await past.text()).toBeTruthy()
    const pastPlate = (await past.json()) as { id: number }
    const cook = await ctx.post("/api/plates", {
      data: {
        date: today,
        slot_id: slot.id,
        components: [{ food_id: chili.id }],
      },
    })
    expect(cook.ok(), await cook.text()).toBeTruthy()
    const cookPlate = (await cook.json()) as {
      id: number
      components: { cook_session_id: number }[]
    }
    const sessionId = cookPlate.components[0].cook_session_id
    const leftovers: number[] = []
    for (const d of [days[4], days[5]]) {
      const res = await ctx.post("/api/plates", {
        data: {
          date: d,
          slot_id: slot.id,
          components: [{ food_id: chili.id, cook_session_id: sessionId }],
        },
      })
      expect(res.ok(), await res.text()).toBeTruthy()
      leftovers.push(((await res.json()) as { id: number }).id)
    }
    // Today's other meals eat the Chili too: today keeps its one cook
    // occasion and has no empty cell, so filling the week needs no second
    // cook today (H2).
    let cells = await windowPlates(ctx, start, today)
    for (const c of cells) {
      if (c.date !== today || c.slot_id === slot.id || c.mode === "none")
        continue
      const res = await ctx.post("/api/plates", {
        data: {
          date: today,
          slot_id: c.slot_id,
          components: [{ food_id: chili.id, cook_session_id: sessionId }],
        },
      })
      expect(res.ok(), await res.text()).toBeTruthy()
    }
    const before = await session(ctx, sessionId)
    cells = await windowPlates(ctx, start, today)
    slotIds = [...new Set(cells.map((c) => c.slot_id))]
    const chiliPlates = cells.filter((c) =>
      leftovers.includes(c.plate?.id ?? 0)
    )
    expect(chiliPlates.map((c) => c.plate!.state)).toEqual([
      "confirmed",
      "confirmed",
    ])

    // Mid-week in that window; no assistant, so the sheet pins the heuristic.
    await page.clock.setFixedTime(new Date(`${today}T12:00:00`))
    await mockSettingValues(page, { "ai.provider": "" })
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/plan?start=${start}`)
    await expect(planCell(page, days[4], slot.id)).not.toHaveAttribute(
      "data-cell-empty",
      ""
    )

    // Keep them: everything stays.
    let sheet = await openAndMark(page, days)
    await expect(sheet.getByTestId("replan-changes")).toContainText(
      `${weekday(days[4])} and ${weekday(days[5])} are now No plate.`
    )
    await expect(sheet.getByTestId("replan-changes")).toContainText(
      `${chili.name} has`
    )
    const bought = sheet.getByTestId("replan-bought")
    await expect(bought).toContainText("These were already bought for")
    // The slot "slot.rpl-<tag>" reads "Rpl <Tag>", lowercased in a cell name.
    for (const d of [days[4], days[5]]) {
      await expect(bought).toContainText(
        `${weekday(d)} rpl ${tag} · ${chili.name}`
      )
    }
    let replanned = false
    page.on("request", (r) => {
      if (r.url().includes("/api/plan/replan?") && r.method() === "POST")
        replanned = true
    })
    await sheet.getByRole("button", { name: "Keep them" }).click()
    await expect(sheet).toHaveCount(0)
    expect(replanned).toBe(false)
    cells = await windowPlates(ctx, start, today)
    for (const id of leftovers)
      expect(cells.some((c) => c.plate?.id === id)).toBe(true)
    expect(await session(ctx, sessionId)).toMatchObject(before)

    // Change them: the Thu/Fri plates go, the spare freezes, drafts fill.
    sheet = await openAndMark(page, days)
    await expect(sheet.getByLabel(/^Freeze \d/)).toBeChecked()
    await sheet.getByLabel("Re-plan the rest of the week").check()
    const post = page.waitForResponse(
      (r) =>
        r.url().includes("/api/plan/replan?") && r.request().method() === "POST"
    )
    await sheet.getByRole("button", { name: "Change them" }).click()
    const res = await post
    expect(res.ok(), await res.text()).toBeTruthy()
    expect(res.request().postDataJSON()).toMatchObject({
      from: today,
      to: end,
      unlock_confirmed: true,
      spare: "freeze",
      regenerate: true,
      chooser: "heuristic",
    })
    const { run_id: runId } = (await res.json()) as { run_id: number | null }
    expect(runId).not.toBeNull()
    await expect(sheet).toHaveCount(0)
    await expect
      .poll(
        async () => {
          const r = await ctx.get(`/api/plan/runs/${runId}`)
          return ((await r.json()) as { status: string }).status
        },
        { timeout: 30_000 }
      )
      .not.toBe("running")
    const run = (await (await ctx.get(`/api/plan/runs/${runId}`)).json()) as {
      status: string
      counts: { plates: number }
    }
    expect(run.status, JSON.stringify(run)).toBe("ok")
    expect(run.counts.plates).toBeGreaterThan(0)
    // The result is a draft overlay on the board.
    await expect(page.getByTestId("plan-summary")).toHaveText(/^Draft · /)

    cells = await windowPlates(ctx, start, today)
    for (const d of [days[4], days[5]]) {
      const c = cells.find((x) => x.date === d && x.slot_id === slot.id)!
      expect(c.mode).toBe("none")
      expect(c.plate).toBeNull()
    }
    const after = await session(ctx, sessionId)
    expect(after.portions).toBe(before.portions)
    expect(after.frozen_portions).toBeGreaterThan(before.frozen_portions)
    // Today's cook plate and the past plate stay confirmed; past days get
    // no drafts; the empty cells from today on are drafts.
    const plates = cells.flatMap((c) => (c.plate ? [c.plate] : []))
    expect(plates.find((p) => p.id === cookPlate.id)?.state).toBe("confirmed")
    expect(plates.find((p) => p.id === pastPlate.id)?.state).toBe("confirmed")
    expect(
      plates.filter((p) => p.date < today && p.id !== pastPlate.id)
    ).toEqual([])
    const drafts = plates.filter((p) => p.state === "draft")
    expect(drafts.length).toBeGreaterThan(0)
    expect(drafts.every((p) => p.date >= today)).toBe(true)
  } finally {
    await page.unrouteAll({ behavior: "ignoreErrors" })
    // Each step on its own, so one failure can't skip the rest.
    const steps: (() => Promise<unknown>)[] = [
      // The generation filled every slot of the window, not just ours.
      () => ctx.delete(`/api/plan?from=${start}&to=${end}`),
      ...[days[4], days[5]].flatMap((d) =>
        slotIds.map((s) => () => ctx.delete(`/api/availability/${d}/${s}`))
      ),
      ...(slotId !== undefined ? [() => cleanupSlot(slotId!)] : []),
      ...(library ? [() => library!.cleanup()] : []),
      () =>
        freezingBefore?.source === "db"
          ? ctx.put("/api/settings/plan.freezing_enabled", {
              data: { value: freezingBefore.value },
            })
          : ctx.delete("/api/settings/plan.freezing_enabled"),
    ]
    for (const step of steps) {
      await step().catch((err: unknown) =>
        console.warn("plan-replan cleanup:", err)
      )
    }
    await ctx.dispose()
  }
})
