import type { APIRequestContext, Page } from "@playwright/test"

import {
  addDays,
  API,
  apiRequest,
  cleanupFood,
  expect,
  mockSettingValues,
  planCell,
  seedLeafFood,
  seedRecipeLibrary,
  setSetting,
  test,
  uid,
} from "./helpers"

// Phase 3 F3: the plan wizard. Runs in global-settings, alone: a real
// generation plans every active slot of its window and uses the shared
// recipes, one test writes the availability defaults and one swaps the
// global ai.fake_script (both restored afterwards).

interface Default {
  weekday: number
  slot_id: number
  mode: "plan" | "routine" | "none"
  preset_id: number | null
}

/** 0 = Monday … 6 = Sunday. */
function weekday(iso: string) {
  return (new Date(`${iso}T00:00:00Z`).getUTCDay() + 6) % 7
}

const DAY = new Intl.DateTimeFormat("en-US", {
  weekday: "long",
  day: "numeric",
  month: "long",
  timeZone: "UTC",
})
/** The wizard grid's name for a date ("Thursday, November 5"). */
const dayName = (iso: string) => DAY.format(new Date(`${iso}T00:00:00Z`))

// The shared DB accumulates recipes, so pin the recipe gate open.
async function stubRecipesReady(page: Page, count = 10) {
  await page.route("**/api/setup", (route) =>
    route.fulfill({
      json: { completed: true, recipe_count: count, recipe_target: 10 },
    })
  )
}

async function json<T>(ctx: APIRequestContext, path: string): Promise<T> {
  const res = await ctx.get(path)
  expect(res.ok(), await res.text()).toBeTruthy()
  return (await res.json()) as T
}

test.describe("plan wizard", () => {
  test("F3-AC1/AC2: Today's banner → Who's home? → Skip → the draft board (AC-1 steps 1–4)", async ({
    page,
  }) => {
    test.slow()
    const tag = uid()
    const ctx = await apiRequest.newContext({ baseURL: API })
    const { window } = await json<{ window: { start: string } }>(
      ctx,
      "/api/plan"
    )
    // Far beyond the windows other specs use (futureWindow: ≤ 50 ahead).
    const start = addDays(window.start, 7 * 60)
    const days = Array.from({ length: 7 }, (_, i) => addDays(start, i))
    const end = days[6]

    const slots = (
      await json<{
        items: { id: number; name_key: string; active: boolean }[]
      }>(ctx, "/api/settings/slots")
    ).items
    const slot = (key: string) => slots.find((s) => s.name_key === key)!.id
    const breakfast = slot("slot.breakfast")
    const lunch = slot("slot.lunch")
    const dinner = slot("slot.dinner")

    const original = (
      await json<{ items: Default[] }>(ctx, "/api/availability/defaults")
    ).items
    const library = await seedRecipeLibrary(tag)
    const oatsFood = await seedLeafFood({
      name: `Oats ${tag}`,
      kcal_100g: 370,
      protein_100g: 13,
      fat_100g: 7,
      carbs_100g: 60,
    })
    const presetRes = await ctx.post("/api/presets", {
      data: {
        name: `Oats ${tag}`,
        plates: [
          {
            slot_id: breakfast,
            components: [{ food_id: oatsFood.id, amount: 80, unit: "g" }],
          },
        ],
      },
    })
    expect(presetRes.ok(), await presetRes.text()).toBeTruthy()
    const preset = (await presetRes.json()) as { id: number }

    // The "Batch cooker" week: Mon–Fri breakfast is the Oats routine,
    // every other meal is planned.
    const res = await ctx.put("/api/availability/defaults", {
      data: {
        items: original.map((d) =>
          d.slot_id === breakfast && d.weekday < 5
            ? { ...d, mode: "routine", preset_id: preset.id }
            : { ...d, mode: "plan", preset_id: null }
        ),
      },
    })
    expect(res.ok(), await res.text()).toBeTruthy()

    const away = [
      { date: days[5], slot: dinner, name: "dinner" },
      { date: days[6], slot: lunch, name: "lunch" },
    ]

    try {
      await stubRecipesReady(page)
      await mockSettingValues(page, { "ai.provider": "" })
      await page.setViewportSize({ width: 1440, height: 900 })
      // The day before the window: Today offers to plan it.
      await page.clock.setFixedTime(new Date(`${addDays(start, -1)}T12:00:00`))

      // 1. Today's banner opens the wizard for the next window.
      await page.goto("/today")
      const banner = page.getByTestId("today-plan-banner")
      await banner.getByRole("link", { name: "Plan next week" }).click()
      await expect(page).toHaveURL(`/plan/new?start=${start}`)

      // 2. Who's home? shows the window prefilled from the defaults.
      const wizard = page.getByTestId("plan-wizard")
      await expect(
        wizard.getByRole("heading", { name: "Who's home?" })
      ).toBeVisible()
      for (const date of days) {
        const name = new RegExp(`^${dayName(date)} breakfast, (.+)$`)
        const cell = wizard.getByRole("button", { name })
        await expect(cell).toHaveAccessibleName(
          weekday(date) < 5
            ? `${dayName(date)} breakfast, Routine · Oats ${tag}`
            : `${dayName(date)} breakfast, Plan`
        )
      }
      for (const a of away) {
        await wizard
          .getByRole("button", { name: `${dayName(a.date)} ${a.name}, Plan` })
          .click()
        await expect(
          wizard.getByRole("button", {
            name: `${dayName(a.date)} ${a.name}, No plate`,
          })
        ).toBeVisible()
      }
      await wizard
        .getByRole("checkbox", { name: "Make this my new default" })
        .check()
      const saved = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/availability/defaults") &&
          r.request().method() === "PUT"
      )
      await wizard.getByRole("button", { name: "Next" }).click()
      expect((await saved).ok()).toBeTruthy()

      // F3-AC2: the defaults changed, the dates got no override.
      const defaults = (
        await json<{ items: Default[] }>(ctx, "/api/availability/defaults")
      ).items
      for (const a of away) {
        expect(
          defaults.find(
            (d) => d.weekday === weekday(a.date) && d.slot_id === a.slot
          )?.mode
        ).toBe("none")
      }
      const cells = (
        await json<{
          items: { date: string; slot_id: number; source: string }[]
        }>(ctx, `/api/availability?from=${start}&to=${end}`)
      ).items
      expect(cells.filter((c) => c.source === "override")).toEqual([])

      // 3. Skip, then the progress checklist, then the board. The first
      // poll waits a moment so the checklist is there to see.
      let delayed = false
      await page.route(/\/api\/plan\/runs\/\d+$/, async (route) => {
        if (!delayed) {
          delayed = true
          await new Promise((r) => setTimeout(r, 1000))
        }
        await route.continue()
      })
      await expect(
        wizard.getByRole("heading", { name: "Anything special this week?" })
      ).toBeVisible()
      const generated = page.waitForResponse(
        (r) =>
          r.url().includes("/api/plan/generate") &&
          r.request().method() === "POST"
      )
      await wizard
        .getByRole("button", { name: "Generate without a note" })
        .click()
      const response = await generated
      expect(response.request().postDataJSON()).toMatchObject({
        chooser: "heuristic",
      })
      const { run_id: runId } = (await response.json()) as { run_id: number }
      await expect(wizard.getByTestId("wizard-generating")).toBeVisible()
      await expect(
        wizard.getByTestId("wizard-generating").getByRole("listitem")
      ).toHaveText([
        /^Reading your week/,
        /^Choosing recipes/,
        /^Checking batches and keep days/,
        /^Saving draft/,
      ])

      // 4. The board: every plan cell holds a dashed draft plate, Mon–Fri
      // breakfast is the Oats routine, the two away cells stay No plate.
      await expect(page.getByTestId("plan-wizard")).toHaveCount(0, {
        timeout: 30_000,
      })
      await expect(page).toHaveURL(new RegExp(`/plan\\?start=${start}`))
      await expect(page.getByTestId("plan-summary")).toHaveText(
        /^Draft · \d+ plates · \d+ cook days?$/
      )
      await expect(page.getByTestId("plan-warnings")).toHaveCount(0)
      for (const date of days) {
        for (const slotId of [breakfast, lunch, dinner]) {
          const cell = planCell(page, date, slotId)
          if (away.some((a) => a.date === date && a.slot === slotId)) {
            await expect(cell).toHaveAttribute("data-cell-mode", "none")
            await expect(cell).toHaveAttribute("data-cell-empty", "")
            continue
          }
          await expect(cell).not.toHaveAttribute("data-cell-empty")
          await expect(cell.getByTestId("plan-cell")).toHaveClass(
            /border-dashed/
          )
          if (slotId === breakfast && weekday(date) < 5) {
            await expect(cell).toContainText(`Oats ${tag}`)
          }
        }
      }

      // The run shows in Insights as a scored heuristic Generate run.
      await page.goto("/settings/assistant/insights")
      const row = page.getByTestId(`run-row-${runId}`)
      await expect(row).toContainText("Generate")
      await expect(row).toContainText("Without the assistant")
      await expect(row).toContainText("Done")
      await expect(row).toContainText(/\d+\s?% covered · \d+ violations?/)
    } finally {
      await ctx.delete(`/api/plan?from=${start}&to=${end}`)
      await ctx.put("/api/availability/defaults", {
        data: { items: original },
      })
      await ctx.delete(`/api/presets/${preset.id}`)
      await library.cleanup()
      await cleanupFood(oatsFood.id)
      await ctx.dispose()
    }
  })

  test("F3-AC4: Generate plan opens the wizard, not the chat", async ({
    page,
  }) => {
    await stubRecipesReady(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/settings")
    await page
      .getByTestId("sidenav")
      .getByTestId("generate-plan-default")
      .click()
    await expect(page).toHaveURL("/plan/new")
    await expect(
      page.getByTestId("plan-wizard").getByRole("heading", {
        name: "Who's home?",
      })
    ).toBeVisible()
    await expect(page.getByTestId("chat-composer-input")).toHaveCount(0)

    // Escape closes it onto the board of the same window.
    await page.keyboard.press("Escape")
    await expect(page.getByTestId("plan-wizard")).toHaveCount(0)
    await expect(page).toHaveURL(/\/plan\?start=\d{4}-\d{2}-\d{2}$/)
  })

  test("below the recipe target /plan/new goes to Recipes and says why", async ({
    page,
  }) => {
    await stubRecipesReady(page, 4)
    await page.goto("/plan/new")
    await expect(page).toHaveURL("/recipes")
    await expect(
      page.getByText("Add 6 more recipes to start planning").first()
    ).toBeVisible()
  })

  test("phones get the wizard as a full page", async ({ page }) => {
    await stubRecipesReady(page)
    await page.setViewportSize({ width: 375, height: 812 })
    await page.goto("/plan/new")
    const wizard = page.getByTestId("plan-wizard")
    await expect(
      wizard.getByRole("heading", { level: 1, name: "Who's home?" })
    ).toBeVisible()
    await expect(page.getByRole("dialog")).toHaveCount(0)
    await expect(page.getByTestId("plan-summary")).toHaveCount(0)

    // Scrolled sideways, no cell shows through either side of the sticky
    // meal column (the scroll box's padding and the border spacing).
    const grid = wizard.getByRole("grid")
    await grid.locator("..").evaluate((el) => (el.scrollLeft = 120))
    const header = grid.getByRole("rowheader").first()
    const box = (await header.boundingBox())!
    const y = box.y + box.height / 2
    for (const x of [box.x - 2, box.x + box.width + 2]) {
      const hit = await page.evaluate(
        ([px, py]) =>
          document.elementFromPoint(px, py)?.closest("[role=rowheader]") !==
          null,
        [x, y]
      )
      expect(hit, `x=${x}`).toBe(true)
    }
  })
})

// F3-AC3: the assistant fails (B7's empty fake script fails every call)
// and "Plan without the assistant" plans heuristically (D11: no automatic
// fallback).
const ERROR_SCRIPT =
  "../frontend/e2e/fixtures/chat-scripts/generate-week-error.json"

test("F3-AC3: an assistant failure offers Plan without the assistant", async ({
  page,
  request,
}) => {
  test.slow()
  const library = await seedRecipeLibrary(uid())
  const plan = await request.get(`${API}/api/plan`)
  const { window } = (await plan.json()) as { window: { start: string } }
  const start = addDays(window.start, 7 * 65)
  await setSetting("ai.fake_script", ERROR_SCRIPT)
  try {
    await stubRecipesReady(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/plan/new?start=${start}`)
    const wizard = page.getByTestId("plan-wizard")
    await wizard.getByRole("button", { name: "Next" }).click()
    await wizard
      .getByRole("button", { name: "Generate without a note" })
      .click()

    // A neutral headline; the run's error says what went wrong.
    const failed = wizard.getByTestId("wizard-failed").getByRole("alert")
    await expect(failed).toContainText("Planning didn't work.", {
      timeout: 30_000,
    })
    await expect(failed).not.toContainText("didn't respond")

    const generated = page.waitForResponse(
      (r) =>
        r.url().includes("/api/plan/generate") &&
        r.request().method() === "POST"
    )
    await wizard
      .getByRole("button", { name: "Plan without the assistant" })
      .click()
    const res = await generated
    expect(res.request().postDataJSON()).toMatchObject({
      start,
      chooser: "heuristic",
    })
    const { run_id: runId } = (await res.json()) as { run_id: number }

    await expect(page.getByTestId("plan-wizard")).toHaveCount(0, {
      timeout: 30_000,
    })
    await expect(page.getByTestId("plan-summary")).toHaveText(/^Draft · /)
    const run = await request.get(`${API}/api/plan/runs/${runId}`)
    expect(await run.json()).toMatchObject({
      chooser: "heuristic",
      status: "ok",
    })
  } finally {
    await request.delete(`${API}/api/settings/ai.fake_script`)
    await request.delete(
      `${API}/api/plan?from=${start}&to=${addDays(start, 6)}`
    )
    await library.cleanup()
  }
})
