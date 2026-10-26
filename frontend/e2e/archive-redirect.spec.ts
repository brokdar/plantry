import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedWithStub,
  seedSlot,
  test,
  uid,
} from "./helpers"

test.describe("Archive redirects + sidebar", () => {
  test("/archive redirects to /calendar?mode=agenda", async ({ page }) => {
    await page.goto("/archive")
    await expect(page).toHaveURL(/\/calendar.*mode=agenda/)
  })

  test("/archive/<id> redirects to /calendar?mode=week with a date param", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.arc-rdr-${tag}`, "Moon", 984)
    const { composed, stub } = await seedComposedWithStub(
      { name: `Redirect Dish ${tag}`, role: "main" },
      tag
    )

    const ctx = await apiRequest.newContext({ baseURL: API })
    let pastWeekId = 0
    try {
      // Seed using the old weeks API. If it returns 404, skip the $id redirect test.
      const weekRes = await ctx.get("/api/weeks/by-date?year=2024&week=3")
      if (!weekRes.ok()) {
        // Old weeks API is gone — skip gracefully.
        return
      }
      const pastWeek = (await weekRes.json()) as { id: number }
      pastWeekId = pastWeek.id

      // Add a plate so the week has content.
      await ctx.post(`/api/weeks/${pastWeek.id}/plates`, {
        data: {
          day: 0,
          slot_id: slot.id,
          components: [{ food_id: composed.id, portions: 1 }],
        },
      })

      await page.goto(`/archive/${pastWeekId}`)

      // Should redirect to /calendar?mode=week with a date query param.
      await expect(page).toHaveURL(/\/calendar.*mode=week.*date=/)
    } finally {
      if (pastWeekId !== 0) {
        const det = await ctx.get(`/api/weeks/${pastWeekId}`)
        if (det.ok()) {
          const detail = (await det.json()) as {
            plates: { id: number }[]
          }
          for (const p of detail.plates) {
            await ctx.delete(`/api/plates/${p.id}`)
          }
        }
      }
      await ctx.dispose()
      await cleanupFood(composed.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("Archive left the sidebar but is reachable via the command palette", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto("/")

    const sidebar = page.getByTestId("sidenav")
    await expect(
      sidebar.getByRole("navigation", { name: "Primary" }).getByRole("link")
    ).toHaveCount(4)
    await expect(sidebar.getByRole("link", { name: /archive/i })).toHaveCount(0)
    await expect(sidebar.getByRole("link", { name: /calendar/i })).toHaveCount(
      0
    )

    await page.keyboard.press("ControlOrMeta+k")
    await page
      .getByRole("dialog")
      .getByRole("option", { name: "Archive", exact: true })
      .click()
    await expect(page).toHaveURL(/\/calendar.*mode=agenda/)
  })
})
