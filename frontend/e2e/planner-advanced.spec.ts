import { cleanupSlot, expect, seedSlot, test, uid } from "./helpers"

test.describe("Planner — advanced flows", () => {
  test("copy-week backend error surfaces as a toast (not window.alert)", async ({
    page,
  }) => {
    const slot = await seedSlot(`slot.plan-err-${uid()}`, "Moon", 999)

    try {
      // Intercept copy endpoint to force a 500.
      await page.route(/\/api\/weeks\/\d+\/copy/, (route) =>
        route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ status: 500, message_key: "error.server" }),
        })
      )

      await page.goto("/")
      await page.getByRole("button", { name: /copy week to next/i }).click()

      // Sonner toast surface shows the error.
      await expect(page.getByText("Something went wrong.")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("week navigator prev/next buttons are clickable without errors", async ({
    page,
  }) => {
    const slot = await seedSlot(`slot.plan-nav-${uid()}`, "Moon", 998)

    try {
      await page.goto("/")
      await expect(
        page.getByRole("button", { name: "Next week" })
      ).toBeVisible()

      await page.getByRole("button", { name: "Next week" }).click()
      await page.getByRole("button", { name: "Previous week" }).click()

      // Header re-rendered (no crash) — grid still present.
      await expect(page.getByTestId("page-header")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
