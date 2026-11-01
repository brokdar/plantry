import type { APIRequestContext, Locator, Page } from "@playwright/test"

import {
  addDays,
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  seedComposedWithStub,
  seedSlot,
  test,
  uid,
} from "./helpers"

// F3: the plan board at /plan (UX §5.5). Each test seeds the AC-1 batch
// (cook on day 2 dinner, leftovers day 3 lunch and dinner) on its own
// slots in its own future window, so parallel specs that clear or fill the
// current window never touch it.

interface PlanWindow {
  start: string
  end: string
  prev_start: string
  next_start: string
}

interface Seeded {
  window: PlanWindow
  lunch: number
  dinner: number
  /** Window dates, day 0 = the shopping day. */
  days: string[]
  portions: number
  chili: string
  cleanup: () => Promise<void>
}

async function api(): Promise<APIRequestContext> {
  return apiRequest.newContext({ baseURL: API })
}

async function anchoredWindow(): Promise<PlanWindow> {
  const ctx = await api()
  const res = await ctx.get("/api/plan")
  expect(res.ok()).toBeTruthy()
  const body = (await res.json()) as { window: PlanWindow }
  await ctx.dispose()
  return body.window
}

/** Seeds AC-1 in this worker's own future window (`futureWindow`), on
 *  two fresh slots. */
async function seedAc1(): Promise<Seeded> {
  const tag = uid()
  const { start, days } = await futureWindow()
  const lunch = await seedSlot(`slot.pb-lunch-${tag}`, "Sun", 901)
  const dinner = await seedSlot(`slot.pb-dinner-${tag}`, "Moon", 902)
  const { composed: chili, stub } = await seedComposedWithStub(
    { name: `Chili ${tag}`, role: "main", reference_portions: 4 },
    tag
  )

  const ctx = await api()
  const plateRes = await ctx.post("/api/plates", {
    data: {
      date: days[1],
      slot_id: dinner.id,
      eaters: 2,
      components: [{ food_id: chili.id, portions: 1 }],
    },
  })
  expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
  const plate = (await plateRes.json()) as {
    components: { cook_session_id: number }[]
  }
  const sessionId = plate.components[0].cook_session_id
  const leftovers = await ctx.post(
    `/api/cook-sessions/${sessionId}/leftovers`,
    {
      data: {
        cells: [
          { date: days[2], slot_id: lunch.id },
          { date: days[2], slot_id: dinner.id },
        ],
      },
    }
  )
  expect(leftovers.ok(), await leftovers.text()).toBeTruthy()
  const session = (await leftovers.json()) as { portions: number }
  const planRes = await ctx.get(`/api/plan?start=${start}`)
  const window = ((await planRes.json()) as { window: PlanWindow }).window
  await ctx.dispose()

  return {
    window,
    lunch: lunch.id,
    dinner: dinner.id,
    days,
    portions: session.portions,
    chili: chili.name,
    cleanup: async () => {
      // Only this test's plates: another test may share the window.
      const c = await api()
      const res = await c.get(`/api/plan?start=${window.start}`)
      const { cells } = (await res.json()) as {
        cells: { slot_id: number; plate: { id: number } | null }[]
      }
      for (const { slot_id, plate } of cells) {
        if (plate && (slot_id === lunch.id || slot_id === dinner.id)) {
          await c.delete(`/api/plates/${plate.id}`)
        }
      }
      for (const date of days) {
        await c.delete(`/api/availability/${date}/${dinner.id}`)
      }
      await c.dispose()
      await cleanupSlot(lunch.id)
      await cleanupSlot(dinner.id)
      await cleanupFood(chili.id)
      await cleanupFood(stub.id)
    },
  }
}

function cell(page: Page, date: string, slotId: number): Locator {
  return page.locator(`[data-cell-key="${date}|${slotId}"]`)
}

async function expectAc1Badges(page: Page, s: Seeded) {
  await expect(cell(page, s.days[1], s.dinner)).toContainText(
    `Cook × ${s.portions}`
  )
  await expect(cell(page, s.days[2], s.lunch)).toContainText("2/3")
  await expect(cell(page, s.days[2], s.dinner)).toContainText("3/3")
}

/** Drags with the mouse in small steps, as dnd-kit's MouseSensor needs. */
async function drag(page: Page, from: Locator, to: Locator, copy = false) {
  // Both cells sit in one meal row; other specs' slots can push it below
  // the fold.
  await from.scrollIntoViewIfNeeded()
  const a = await from.getByTestId("plan-cell").boundingBox()
  const b = await to.getByTestId("plan-cell").boundingBox()
  if (!a || !b) throw new Error("cell not visible")
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2)
  if (copy) await page.keyboard.down("Control")
  await page.mouse.down()
  if (copy) await page.keyboard.up("Control")
  await page.mouse.move(a.x + a.width / 2 + 10, a.y + a.height / 2, {
    steps: 3,
  })
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 12 })
  // dnd-kit settles the drop target a frame after the move.
  await expect(to).toHaveAttribute("data-over", "")
  await page.mouse.up()
}

test.describe("Plan board", () => {
  test("shows the AC-1 batch with its badges and cook strip; the strip highlights the batch", async ({
    page,
  }) => {
    const s = await seedAc1()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.window.start}`)

      const grid = page.getByRole("grid")
      await expect(grid).toBeVisible()
      await expectAc1Badges(page, s)

      const strip = page.getByRole("region", { name: "Cook days" })
      const entry = strip.getByRole("button", {
        name: new RegExp(`· ${s.chili} × ${s.portions}$`),
      })
      await expect(entry).toBeVisible()
      await expect(entry).toHaveCount(1)

      await entry.locator("[data-batch-badge]").hover()
      await expect(grid.locator("[data-highlighted]")).toHaveCount(3)
    } finally {
      await s.cleanup()
    }
  })

  test("the phone day list shows the same plates and badges", async ({
    page,
  }) => {
    const s = await seedAc1()
    try {
      await page.setViewportSize({ width: 375, height: 800 })
      await page.goto(`/plan?start=${s.window.start}`)

      await expect(page.getByRole("grid")).toHaveCount(0)
      await expectAc1Badges(page, s)
      await expect(
        page.getByRole("region", { name: "Cook days" })
      ).toContainText(`× ${s.portions}`)
    } finally {
      await s.cleanup()
    }
  })

  test("the German toolbar fits a 375 px phone", async ({ page }) => {
    // German for this page only: the shell applies the profile's locale.
    await page.route("**/api/profile", async (route) => {
      if (route.request().method() !== "GET") return route.fallback()
      const res = await route.fetch()
      await route.fulfill({
        response: res,
        json: { ...(await res.json()), locale: "de" },
      })
    })
    await page.setViewportSize({ width: 375, height: 800 })
    await page.goto("/plan")

    const more = page.getByRole("button", {
      name: "Weitere Aktionen für den Plan",
    })
    await expect(more).toBeInViewport({ ratio: 1 })
    await expect(
      page.getByRole("button", { name: "Assistent fragen" })
    ).toBeVisible()
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth
      )
    ).toBeLessThanOrEqual(0)
  })

  test("a tablet board shows part of the fifth day as a scroll hint", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 820, height: 1000 })
    await page.goto("/plan")

    const fifth = page.getByRole("columnheader").nth(5)
    await expect(fifth).toBeInViewport()
    await expect(fifth).not.toBeInViewport({ ratio: 1 })
  })

  test("opens the anchored window; the arrows and This week move between windows", async ({
    page,
  }) => {
    const anchored = await anchoredWindow()
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/plan")

    const heading = page.getByRole("heading", { level: 1 })
    await expect(page.getByRole("grid")).toBeVisible()
    const anchoredTitle = await heading.textContent()
    await expect(
      page.getByRole("link", { name: "This week", exact: true })
    ).toHaveAttribute("aria-current", "page")
    // The first column is the window's first day (the shopping day).
    const firstDay = Number(anchored.start.slice(8))
    await expect(page.getByRole("columnheader").nth(1)).toContainText(
      String(firstDay)
    )

    await page.getByRole("link", { name: "Next week" }).click()
    await expect(page).toHaveURL(`/plan?start=${anchored.next_start}`)
    await expect(heading).not.toHaveText(anchoredTitle ?? "")
    await page.getByRole("link", { name: "Next week" }).click()
    await expect(page).toHaveURL(
      `/plan?start=${addDays(anchored.next_start, 7)}`
    )
    await page.getByRole("link", { name: "Previous week" }).click()
    await expect(page).toHaveURL(`/plan?start=${anchored.next_start}`)

    // This week is the window holding today (the e2e anchor is today).
    await page.getByRole("link", { name: "This week", exact: true }).click()
    await expect(page).toHaveURL(`/plan?start=${anchored.start}`)
    await expect(heading).toHaveText(anchoredTitle ?? "")

    await page.getByRole("link", { name: "Previous week" }).click()
    await expect(page).toHaveURL(`/plan?start=${anchored.prev_start}`)
  })

  test("dragging moves a plate; a drop onto No plate is refused with a toast", async ({
    page,
  }) => {
    const s = await seedAc1()
    const ctx = await api()
    // Day 4 dinner is No plate.
    const none = await ctx.put(`/api/availability/${s.days[4]}/${s.dinner}`, {
      data: { mode: "none" },
    })
    expect(none.ok(), await none.text()).toBeTruthy()
    await ctx.dispose()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.window.start}`)
      await expectAc1Badges(page, s)

      // Monday dinner (day 2) → Tuesday dinner (day 3).
      const moved = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+$/.test(r.url()) && r.request().method() === "PUT"
      )
      await drag(
        page,
        cell(page, s.days[2], s.dinner),
        cell(page, s.days[3], s.dinner)
      )
      expect((await moved).ok()).toBeTruthy()
      await expect(cell(page, s.days[3], s.dinner)).toContainText("3/3")
      await expect(cell(page, s.days[2], s.dinner)).toContainText("Add")

      // Tuesday dinner → the No plate cell: refused before any request.
      let requested = false
      page.on("request", (r) => {
        if (r.method() === "PUT" && /\/api\/plates\//.test(r.url())) {
          requested = true
        }
      })
      await drag(
        page,
        cell(page, s.days[3], s.dinner),
        cell(page, s.days[4], s.dinner)
      )
      await expect(
        page.getByText("This meal is marked No plate.")
      ).toBeVisible()
      await expect(cell(page, s.days[3], s.dinner)).toContainText("3/3")
      expect(requested).toBe(false)

      // Ctrl-drag copies the cook plate as a new fresh meal (its own
      // batch); the original keeps its batch.
      const copied = page.waitForResponse(
        (r) =>
          r.url().includes("/api/plan/restore") &&
          r.request().method() === "POST"
      )
      await drag(
        page,
        cell(page, s.days[1], s.dinner),
        cell(page, s.days[5], s.dinner),
        true
      )
      expect((await copied).ok()).toBeTruthy()
      await expect(cell(page, s.days[5], s.dinner)).toContainText("Cook × 2")
      await expect(cell(page, s.days[1], s.dinner)).toContainText(
        `Cook × ${s.portions}`
      )
    } finally {
      await s.cleanup()
    }
  })

  test("dragging the cook plate moves the batch's cook cell and its strip day (D13)", async ({
    page,
  }) => {
    const s = await seedAc1()
    const weekday = (iso: string) =>
      new Intl.DateTimeFormat("en", {
        weekday: "short",
        timeZone: "UTC",
      }).format(new Date(`${iso}T00:00:00Z`))
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.window.start}`)
      await expectAc1Badges(page, s)
      const strip = page.getByRole("region", { name: "Cook days" })
      const entry = (iso: string) =>
        strip.getByRole("button", {
          name: `${weekday(iso)} · ${s.chili} × ${s.portions}`,
        })
      await expect(entry(s.days[1])).toBeVisible()

      // The cook plate, day 1 dinner → day 0 dinner.
      const moved = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+$/.test(r.url()) && r.request().method() === "PUT"
      )
      await drag(
        page,
        cell(page, s.days[1], s.dinner),
        cell(page, s.days[0], s.dinner)
      )
      expect((await moved).ok()).toBeTruthy()

      await expect(cell(page, s.days[0], s.dinner)).toContainText(
        `Cook × ${s.portions}`
      )
      await expect(cell(page, s.days[1], s.dinner)).toContainText("Add")
      await expect(cell(page, s.days[2], s.lunch)).toContainText("2/3")
      await expect(cell(page, s.days[2], s.dinner)).toContainText("3/3")
      await expect(entry(s.days[0])).toBeVisible()
      await expect(entry(s.days[1])).toHaveCount(0)
    } finally {
      await s.cleanup()
    }
  })

  test("Clear this week removes every plate and Undo brings them back with the batch", async ({
    page,
  }) => {
    const s = await seedAc1()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.window.start}`)
      await expectAc1Badges(page, s)

      await page.getByRole("button", { name: "More plan actions" }).click()
      const cleared = page.waitForResponse(
        (r) =>
          r.url().includes("/api/plan?") && r.request().method() === "DELETE"
      )
      await page.getByRole("menuitem", { name: "Clear this week" }).click()
      expect((await cleared).ok()).toBeTruthy()
      await expect(page.locator("[data-batch-badge]")).toHaveCount(0)
      await expect(page.getByTestId("plan-empty")).toBeVisible()

      const restored = page.waitForResponse(
        (r) =>
          r.url().includes("/api/plan/restore") &&
          r.request().method() === "POST"
      )
      await page.getByRole("button", { name: "Undo" }).click()
      expect((await restored).ok()).toBeTruthy()
      await expectAc1Badges(page, s)
      await expect(
        page.getByRole("region", { name: "Cook days" })
      ).toContainText(`× ${s.portions}`)
    } finally {
      await s.cleanup()
    }
  })
})
