import {
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  openCellSheet,
  seedLeafFood,
  seedSlot,
  stageFood,
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
  const { start, days } = await futureWindow()

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

    await page.goto(`/plan?start=${start}`)
    const sheet = await openCellSheet(page, days[0], slot.id)
    await sheet.getByRole("button", { name: "Add food" }).click()
    await stageFood(sheet, `Toast food ${tag}`, "Ingredients")
    await sheet.getByTestId("food-picker-commit").click()

    // Toast rendered by sonner — text taken from i18n.
    await expect(page.getByText("Something went wrong.")).toBeVisible()
  } finally {
    await cleanupFood(food.id)
    await cleanupSlot(slot.id)
  }
})
