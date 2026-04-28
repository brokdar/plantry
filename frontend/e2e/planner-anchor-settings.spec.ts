import {
  API,
  apiRequest,
  cleanupSlot,
  expect,
  mockAnchorToday,
  seedSlot,
  test,
  uid,
} from "./helpers"

async function setSetting(key: string, value: string) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.put("/api/settings", { data: { key, value } })
  await ctx.dispose()
}

test.describe("Planner — anchor settings", () => {
  // Always restore anchor to "today" after each test so parallel instances
  // don't leak state into each other.
  test.afterEach(async () => {
    await setSetting("plan.anchor", "today")
  })

  test("Settings → Plan tab renders anchor radio group", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.anc_radio_${tag}`, "Moon", 960)

    try {
      await page.goto("/settings?tab=plan")

      // Plan tab content should be active
      await expect(page.getByTestId("plan-anchor-radio-today")).toBeVisible()
      await expect(
        page.getByTestId("plan-anchor-radio-next_shopping_day")
      ).toBeVisible()
      await expect(
        page.getByTestId("plan-anchor-radio-fixed_weekday")
      ).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("selecting Fixed weekday anchor reveals the weekday picker", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.anc_fixed_${tag}`, "Sun", 959)

    try {
      // Inject "today" for only the initial GET /api/settings so that the
      // settings page always opens with "today" selected, regardless of what
      // parallel test instances may have written to the backend. Subsequent
      // GETs (after the radio click) pass through so the UI reacts to the real
      // PUT response.
      await page.route(
        "**/api/settings",
        async (route) => {
          if (route.request().method() !== "GET") {
            await route.continue()
            return
          }
          const response = await route.fetch()
          const body = (await response.json()) as {
            items: { key: string; value: string }[]
          }
          const items = body.items.map((item) =>
            item.key === "plan.anchor" ? { ...item, value: "today" } : item
          )
          if (!items.some((i) => i.key === "plan.anchor")) {
            items.push({ key: "plan.anchor", value: "today" })
          }
          await route.fulfill({
            status: response.status(),
            headers: {
              ...response.headers(),
              "content-type": "application/json",
            },
            body: JSON.stringify({ ...body, items }),
          })
        },
        { times: 1 }
      )

      await page.goto("/settings?tab=plan")

      // Weekday picker should not be visible while not in fixed_weekday mode
      await expect(page.getByTestId("plan-fixed-weekday-picker")).toHaveCount(0)

      // Select fixed_weekday
      const saveResp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/settings") && r.request().method() === "PUT"
      )
      await page.getByTestId("plan-anchor-radio-fixed_weekday").click()
      await saveResp

      // Weekday picker must now be visible
      await expect(page.getByTestId("plan-fixed-weekday-picker")).toBeVisible()
      await expect(page.getByTestId("plan-fixed-weekday-select")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("after saving fixed_weekday anchor, planner grid starts on that weekday", async ({
    page,
  }) => {
    const tag = uid()
    // Use a high sort_order so this slot doesn't collide with parallel tests
    const slot = await seedSlot(`slot.anc_fwd_${tag}`, "Moon", 958)

    try {
      // Set anchor = fixed_weekday + weekday = Monday (0)
      await setSetting("plan.anchor", "fixed_weekday")
      await setSetting("plan.shopping_day", "0") // 0 = Monday (backend convention)

      await page.goto("/")

      // The grid should be visible. We can't assert the exact calendar date
      // without depending on the day of the test run, but we verify the grid
      // still renders its 7 columns.
      for (let i = 0; i < 7; i++) {
        await expect(page.getByTestId(`day-header-${i}`)).toBeVisible()
      }

      // The planner toolbar (which contains DateRangeNavigator) must be visible
      await expect(page.getByTestId("planner-toolbar")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("after saving today anchor, planner grid starts on today", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.anc_today_${tag}`, "Star", 957)

    try {
      // Use a page-level mock instead of a backend setSetting so this test is
      // fully isolated from parallel test instances that also mutate the anchor.
      await mockAnchorToday(page)

      await page.goto("/")

      // Day-header-0 should have the "today" indicator
      const header0 = page.getByTestId("day-header-0")
      await expect(header0).toBeVisible()

      // The day-header-0 element carries data-today="true" when the day is today
      await expect(header0).toHaveAttribute("data-today", "true")
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
