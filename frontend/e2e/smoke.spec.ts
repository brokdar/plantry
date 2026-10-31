import { expect, test } from "./helpers"

test("home page loads with sidebar brand and no console errors", async ({
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

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto("/")

  // All routes render the default sidebar with the brand link.
  await expect(page.getByTestId("sidenav")).toBeVisible()
  await expect(page.getByTestId("sidenav-brand")).toBeVisible()

  // After setup, `/` opens Today with its verdict and the week strip.
  await expect(page).toHaveURL(/\/today$/)
  await expect(page.getByTestId("today-verdict")).toBeVisible()
  await expect(
    page.getByRole("navigation", { name: "This week" })
  ).toBeVisible()

  expect(consoleErrors, `console errors: ${consoleErrors.join("\n")}`).toEqual(
    []
  )
})
