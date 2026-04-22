import { cleanupSlot, expect, seedSlot, test, uid } from "./helpers"

// Cross-cutting test for the toast provider: intercepting a mutation with an
// error response must surface a destructive toast (not window.alert, which
// the failOnDialog fixture would fail on).

test("mutation error shows a destructive toast with i18n message", async ({
  page,
}) => {
  const slot = await seedSlot(`slot.toast-${uid()}`, "Moon", 995)

  try {
    await page.route(/\/api\/weeks\/\d+\/copy/, (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({
          status: 500,
          message_key: "error.server",
        }),
      })
    )

    await page.goto("/")
    await page.getByRole("button", { name: /copy week to next/i }).click()

    // Toast rendered by sonner — text taken from i18n.
    await expect(page.getByText("Something went wrong.")).toBeVisible()
  } finally {
    await cleanupSlot(slot.id)
  }
})
