import { test, expect } from "@playwright/test"

test("home page loads with Plantry brand and no console errors", async ({
  page,
}) => {
  const consoleErrors: string[] = []
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text())
    }
  })
  page.on("pageerror", (err) => {
    consoleErrors.push(err.message)
  })

  await page.goto("/")

  await expect(
    page.getByRole("link", { name: /plantry/i }).first()
  ).toBeVisible()
  // The home route renders the planner; before slots are configured the user
  // sees the empty-state heading. After they're configured, the planner
  // heading appears. Either signals a successful render.
  await expect(
    page
      .getByRole("heading", {
        name: /(weekly planner|set up your time slots first)/i,
      })
      .first()
  ).toBeVisible()

  expect(consoleErrors, `console errors: ${consoleErrors.join("\n")}`).toEqual(
    []
  )
})
