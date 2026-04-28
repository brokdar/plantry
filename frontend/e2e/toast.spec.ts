import {
  cleanupFood,
  cleanupSlot,
  expect,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// Cross-cutting test for the toast provider: intercepting a mutation with an
// error response must surface a destructive toast (not window.alert, which
// the failOnDialog fixture would fail on).

test("mutation error shows a destructive toast with i18n message", async ({
  page,
}) => {
  const tag = uid()
  const slot = await seedSlot(`slot.toast-${tag}`, "Moon", 995)
  const food = await seedLeafFood({ name: `Toast food ${tag}` })

  try {
    // Intercept plate creation to force a 500.
    await page.route("**/api/plates", async (route) => {
      if (route.request().method() === "POST") {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ status: 500, message_key: "error.server" }),
        })
      } else {
        await route.continue()
      }
    })

    await page.goto("/")
    const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
    await cell.getByRole("button", { name: /plan meal/i }).click()
    const sheet = page.getByRole("dialog")
    await expect(sheet).toBeVisible()
    await sheet.locator("input").first().fill(`Toast food ${tag}`)
    await sheet
      .getByRole("button", { name: new RegExp(`Toast food ${tag}`) })
      .click()

    // Toast rendered by sonner — text taken from i18n.
    await expect(page.getByText("Something went wrong.")).toBeVisible()
  } finally {
    await cleanupFood(food.id)
    await cleanupSlot(slot.id)
  }
})
