import {
  API,
  apiRequest,
  cleanupComponent,
  cleanupIngredient,
  cleanupSlot,
  expect,
  seedComponent,
  seedIngredient,
  seedSlot,
  test,
  uid,
} from "./helpers"

async function seedPastWeekWithPlate(year: number, week: number) {
  const tag = uid()
  const slot = await seedSlot(`slot.copy-${tag}`, "Moon", 991)
  const ing = await seedIngredient({ name: `Ing ${tag}`, kcal_100g: 100 })
  const comp = await seedComponent({
    name: `Copy Dish ${tag}`,
    role: "main",
    ingredients: [
      {
        ingredient_id: ing.id,
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
      components: [{ component_id: comp.id, portions: 1 }],
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
      await cleanupComponent(comp.id)
      await cleanupIngredient(ing.id)
      await cleanupSlot(slot.id)
    },
  }
}

test.describe("Archive detail + copy-to-current", () => {
  test("detail route renders meals in a read-only grid", async ({ page }) => {
    const { pastWeekId, componentName, cleanup } = await seedPastWeekWithPlate(
      2024,
      1
    )

    try {
      await page.goto(`/archive/${pastWeekId}`)
      await expect(page.getByText(componentName)).toBeVisible()
      // No add-a-meal affordance in the read-only grid.
      await expect(
        page.getByRole("button", { name: /Add a meal/i })
      ).toHaveCount(0)
    } finally {
      await cleanup()
    }
  })

  test("copy-to-current from archive list posts and navigates to planner", async ({
    page,
  }) => {
    const { pastWeekId, cleanup } = await seedPastWeekWithPlate(2024, 2)

    try {
      await page.goto("/archive")
      const copyBtn = page.getByTestId(`copy-to-current-list-${pastWeekId}`)
      await expect(copyBtn).toBeVisible()
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

  test("copy-to-current from archive detail fires the same flow", async ({
    page,
  }) => {
    const { pastWeekId, cleanup } = await seedPastWeekWithPlate(2024, 3)

    try {
      await page.goto(`/archive/${pastWeekId}`)
      await page.getByTestId(`copy-to-current-detail-${pastWeekId}`).click()

      const copyResp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/weeks/${pastWeekId}/copy`) &&
          r.request().method() === "POST"
      )
      await page
        .getByRole("dialog")
        .getByRole("button", { name: /copy to current/i })
        .click()
      const res = await copyResp
      expect(res.status()).toBeLessThan(400)

      await expect(page).toHaveURL("/")
    } finally {
      await cleanup()
    }
  })

  test("backend error surfaces as a toast (not window.alert)", async ({
    page,
  }) => {
    const { pastWeekId, cleanup } = await seedPastWeekWithPlate(2024, 4)

    try {
      // Intercept the copy endpoint to return 500.
      await page.route(`**/api/weeks/${pastWeekId}/copy`, (route) =>
        route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ status: 500, message_key: "error.server" }),
        })
      )

      await page.goto("/archive")
      await page.getByTestId(`copy-to-current-list-${pastWeekId}`).click()
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
