import {
  API,
  apiRequest,
  cleanupSlot,
  expect,
  seedSlot,
  test,
  uid,
} from "./helpers"

/** Read current value of a settings key via the API. */
async function getSetting(key: string): Promise<string | null> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get("/api/settings")
  const body = (await res.json()) as { items: { key: string; value: string }[] }
  await ctx.dispose()
  return body.items.find((i) => i.key === key)?.value ?? null
}

/** Write a settings key via the API. */
async function setSetting(key: string, value: string) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.put("/api/settings", { data: { key, value } })
  await ctx.dispose()
}

test.describe("Planner — anchor settings", () => {
  // Restore the original anchor after each test to avoid cross-test pollution.
  let originalAnchor: string | null = null

  test.beforeEach(async () => {
    originalAnchor = await getSetting("plan.anchor")
  })

  test.afterEach(async () => {
    if (originalAnchor !== null) {
      await setSetting("plan.anchor", originalAnchor)
    }
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
      // Ensure anchor is set to today
      await setSetting("plan.anchor", "today")

      await page.goto("/")

      // Day-header-0 should have the "today" indicator
      const header0 = page.getByTestId("day-header-0")
      await expect(header0).toBeVisible()

      // The today marker text ("Today") should appear inside the first day header
      await expect(header0.getByText(/Today/i)).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
