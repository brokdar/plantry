import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

/**
 * Returns the ISO 8601 {year, week} for the given date.
 * The ISO week containing 4 Jan is week 1 of that year.
 */
function isoYearWeek(d: Date): { year: number; week: number } {
  const day = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()))
  // Set to nearest Thursday: current date + 4 - current ISO day (Mon=1..Sun=7)
  const dayOfWeek = day.getUTCDay() || 7 // convert Sun(0) to 7
  day.setUTCDate(day.getUTCDate() + 4 - dayOfWeek)
  const yearStart = new Date(Date.UTC(day.getUTCFullYear(), 0, 1))
  const week = Math.ceil(
    ((day.getTime() - yearStart.getTime()) / 86_400_000 + 1) / 7
  )
  return { year: day.getUTCFullYear(), week }
}

async function seedPastWeekWithPlate(year: number, week: number) {
  const tag = uid()
  const slot = await seedSlot(`slot.copy-${tag}`, "Moon", 991)
  const ing = await seedLeafFood({ name: `Ing ${tag}`, kcal_100g: 100 })
  const comp = await seedComposedFood({
    name: `Copy Dish ${tag}`,
    role: "main",
    children: [
      {
        child_id: ing.id,
        amount: 100,
        unit: "g",
        grams: 100,
        sort_order: 0,
      },
    ],
  })

  const ctx = await apiRequest.newContext({ baseURL: API })
  const weekRes = await ctx.get(`/api/weeks/by-date?year=${year}&week=${week}`)
  expect(weekRes.ok()).toBeTruthy()
  const pastWeek = (await weekRes.json()) as { id: number }

  const plateRes = await ctx.post(`/api/weeks/${pastWeek.id}/plates`, {
    data: {
      day: 0,
      slot_id: slot.id,
      components: [{ food_id: comp.id, portions: 1 }],
    },
  })
  expect(plateRes.ok()).toBeTruthy()
  await ctx.dispose()

  return {
    pastWeekId: pastWeek.id,
    componentName: `Copy Dish ${tag}`,
    cleanup: async () => {
      const ctx2 = await apiRequest.newContext({ baseURL: API })
      const det = await ctx2.get(`/api/weeks/${pastWeek.id}`)
      if (det.ok()) {
        const detail = (await det.json()) as { plates: { id: number }[] }
        for (const p of detail.plates) {
          await ctx2.delete(`/api/plates/${p.id}`)
        }
      }
      await ctx2.dispose()
      await cleanupFood(comp.id)
      await cleanupFood(ing.id)
      await cleanupSlot(slot.id)
    },
  }
}

async function seedRecentPlate(daysAgo: number) {
  const tag = uid()
  const slot = await seedSlot(`slot.copy-${tag}`, "Moon", 991)
  const ing = await seedLeafFood({ name: `Ing ${tag}`, kcal_100g: 100 })
  const comp = await seedComposedFood({
    name: `Copy Dish ${tag}`,
    role: "main",
    children: [
      {
        child_id: ing.id,
        amount: 100,
        unit: "g",
        grams: 100,
        sort_order: 0,
      },
    ],
  })

  const d = new Date()
  d.setDate(d.getDate() - daysAgo)
  const date = d.toISOString().slice(0, 10)

  const ctx = await apiRequest.newContext({ baseURL: API })
  const plateRes = await ctx.post("/api/plates", {
    data: { date, slot_id: slot.id },
  })
  expect(plateRes.ok()).toBeTruthy()
  const plate = (await plateRes.json()) as { id: number; week_id: number }

  const compRes = await ctx.post(`/api/plates/${plate.id}/components`, {
    data: { food_id: comp.id, portions: 1 },
  })
  expect(compRes.ok()).toBeTruthy()
  await ctx.dispose()

  return {
    pastWeekId: plate.week_id,
    componentName: `Copy Dish ${tag}`,
    cleanup: async () => {
      const ctx2 = await apiRequest.newContext({ baseURL: API })
      await ctx2.delete(`/api/plates/${plate.id}`)
      await ctx2.dispose()
      await cleanupFood(comp.id)
      await cleanupFood(ing.id)
      await cleanupSlot(slot.id)
    },
  }
}

test.describe("Archive detail + copy-to-current", () => {
  test("detail route redirects to /calendar?mode=week with the week's monday", async ({
    page,
  }) => {
    // Use a week ~14 days ago so the plate is recent enough to be visible if
    // needed, but we only assert the URL redirect here (food-name-in-week-view
    // is out of scope for this archive-redirect spec).
    const twoWeeksAgo = new Date()
    twoWeeksAgo.setDate(twoWeeksAgo.getDate() - 14)
    const { year, week } = isoYearWeek(twoWeeksAgo)
    const { pastWeekId, cleanup } = await seedPastWeekWithPlate(year, week)

    try {
      await page.goto(`/archive/${pastWeekId}`)
      // Should redirect to /calendar?mode=week with a date param
      await expect(page).toHaveURL(/\/calendar.*mode=week.*date=/)
    } finally {
      await cleanup()
    }
  })

  test("/archive list redirects to /calendar?mode=agenda", async ({ page }) => {
    await page.goto("/archive")
    await expect(page).toHaveURL(/\/calendar.*mode=agenda/)

    // Agenda list renders (at least the wrapper is present).
    await expect(
      page.locator("main, [role='main'], .mx-auto").first()
    ).toBeVisible()
  })

  test("copy-to-current from agenda view posts and navigates to planner", async ({
    page,
  }) => {
    // Seed a plate 20 days ago — within the default 60-day agenda window.
    const { pastWeekId, cleanup } = await seedRecentPlate(20)

    try {
      const agendaResp = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.goto("/calendar?mode=agenda")
      await agendaResp
      const copyBtn = page.getByTestId(`copy-to-current-agenda-${pastWeekId}`)
      await expect(copyBtn).toBeVisible({ timeout: 15000 })
      await copyBtn.click()

      // Confirm dialog opens.
      const confirmBtn = page
        .getByRole("dialog")
        .getByRole("button", { name: /copy to current/i })
      await expect(confirmBtn).toBeVisible()

      const copyResp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/weeks/${pastWeekId}/copy`) &&
          r.request().method() === "POST"
      )
      await confirmBtn.click()
      const res = await copyResp
      expect(res.status()).toBeLessThan(400)

      // Navigates to planner.
      await expect(page).toHaveURL("/")
    } finally {
      await cleanup()
    }
  })

  test("backend error surfaces as a toast (not window.alert)", async ({
    page,
  }) => {
    // Seed a plate 25 days ago — within the default 60-day agenda window.
    const { pastWeekId, cleanup } = await seedRecentPlate(25)

    try {
      // Intercept the copy endpoint to return 500.
      await page.route(`**/api/weeks/${pastWeekId}/copy`, (route) =>
        route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ status: 500, message_key: "error.server" }),
        })
      )

      const agendaResp2 = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.goto("/calendar?mode=agenda")
      await agendaResp2
      await expect(
        page.getByTestId(`copy-to-current-agenda-${pastWeekId}`)
      ).toBeVisible({ timeout: 15000 })
      await page.getByTestId(`copy-to-current-agenda-${pastWeekId}`).click()
      await page
        .getByRole("dialog")
        .getByRole("button", { name: /copy to current/i })
        .click()

      // Toast appears with the error copy.
      await expect(page.getByText("Something went wrong.")).toBeVisible()
    } finally {
      await cleanup()
    }
  })
})
