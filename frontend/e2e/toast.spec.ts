import {
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  openCellSheet,
  seedLeafFood,
  seedPlateWithComponent,
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

function overlaps(a: Box, b: Box) {
  return (
    a.x < b.x + b.width &&
    b.x < a.x + a.width &&
    a.y < b.y + b.height &&
    b.y < a.y + a.height
  )
}

type Box = { x: number; y: number; width: number; height: number }

// Toast placement (DoD): bottom-right, so a toast covers neither the board
// heading on a phone nor the header actions of the next page (the accept
// toast follows the user from /plan to /shop and its Copy as text).
test("toasts sit clear of the board heading at 375 and of Shop's Copy as text at 1440", async ({
  page,
}) => {
  const tag = uid()
  const slot = await seedSlot(`slot.toastpos-${tag}`, "Moon", 996)
  const food = await seedLeafFood({ name: `Toast pos ${tag}` })
  const { start, days } = await futureWindow()
  try {
    // Something to buy, so Shop shows Copy as text.
    await seedPlateWithComponent(days[1], slot.id, {
      food_id: food.id,
      amount: 200,
      unit: "g",
    })
    await page.route("**/api/plates", async (route) => {
      if (route.request().method() !== "POST") return route.continue()
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ status: 500, message_key: "error.server" }),
      })
    })
    const failAdd = async () => {
      const sheet = await openCellSheet(page, days[0], slot.id)
      await sheet.getByRole("button", { name: "Add food" }).click()
      await stageFood(sheet, `Toast pos ${tag}`, "Ingredients")
      await sheet.getByTestId("food-picker-commit").click()
      const toast = page.locator("[data-sonner-toast]").first()
      await expect(toast).toBeVisible()
      await page.keyboard.press("Escape")
      return toast
    }

    await page.setViewportSize({ width: 375, height: 812 })
    await page.goto(`/plan?start=${start}`)
    let toast = await failAdd()
    const heading = (await page
      .getByRole("heading", { level: 1 })
      .boundingBox())!
    const tabBar = (await page
      .getByRole("navigation", { name: /primary|main/i })
      .last()
      .boundingBox())!
    let box = (await toast.boundingBox())!
    expect(overlaps(box, heading)).toBe(false)
    expect(box.y + box.height).toBeLessThanOrEqual(tabBar.y)

    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/plan?start=${start}`)
    toast = await failAdd()
    // Client navigation keeps the toast, as after Accept and shop. Shop's
    // default range is today's, so point its list at the seeded window.
    await page.route("**/api/shopping-list?*", async (route) => {
      const u = new URL(route.request().url())
      u.searchParams.set("from", days[0])
      u.searchParams.set("to", days[days.length - 1])
      await route.continue({ url: u.toString() })
    })
    await page.getByRole("link", { name: "Shop" }).first().click()
    const copy = page.getByRole("button", { name: "Copy as text" })
    await expect(copy).toBeVisible()
    await expect(toast).toBeVisible()
    box = (await toast.boundingBox())!
    expect(overlaps(box, (await copy.boundingBox())!)).toBe(false)
    expect(box.y).toBeGreaterThan(900 / 2)
  } finally {
    await cleanupSlot(slot.id)
    await cleanupFood(food.id)
  }
})
