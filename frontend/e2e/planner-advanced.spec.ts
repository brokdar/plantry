import {
  cleanupFood,
  cleanupSlot,
  expect,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

test.describe("Planner — advanced flows", () => {
  test("plate-creation backend error surfaces as a toast (not window.alert)", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.plan-err-${tag}`, "Moon", 999)
    const food = await seedLeafFood({ name: `Err food ${tag}` })

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
      await sheet.locator("input").first().fill(`Err food ${tag}`)
      await sheet
        .getByRole("button", { name: new RegExp(`Err food ${tag}`) })
        .click()

      // Sonner toast surface shows the error.
      await expect(page.getByText("Something went wrong.")).toBeVisible()
    } finally {
      await cleanupFood(food.id)
      await cleanupSlot(slot.id)
    }
  })

  test("window navigator prev/next buttons are clickable without errors", async ({
    page,
  }) => {
    const slot = await seedSlot(`slot.plan-nav-${uid()}`, "Moon", 998)

    try {
      await page.goto("/")
      await expect(page.getByRole("button", { name: /Next 7/i })).toBeVisible()

      await page.getByRole("button", { name: /Next 7/i }).click()
      await page.getByRole("button", { name: /Previous 7/i }).click()

      // Header re-rendered (no crash) — grid still present.
      await expect(page.getByTestId("page-header")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
