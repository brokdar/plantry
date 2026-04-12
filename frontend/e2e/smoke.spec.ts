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
  await expect(
    page.getByRole("heading", { name: /welcome to plantry/i })
  ).toBeVisible()

  expect(consoleErrors, `console errors: ${consoleErrors.join("\n")}`).toEqual(
    []
  )
})
