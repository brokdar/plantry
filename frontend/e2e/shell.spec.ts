import { expect, test } from "./helpers"

test.describe("AppShell", () => {
  test("desktop sidebar renders brand and all primary nav items", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/settings")

    const sidebar = page.getByTestId("sidenav")
    await expect(sidebar).toBeVisible()
    await expect(sidebar.getByTestId("sidenav-brand")).toBeVisible()

    for (const label of [
      /weekly planner/i,
      /recipes/i,
      /pantry/i,
      /past weeks/i,
      /settings/i,
    ]) {
      await expect(sidebar.getByRole("link", { name: label })).toBeVisible()
    }

    await expect(sidebar.getByTestId("generate-plan-default")).toBeVisible()
  })

  test("planner route uses the same default sidebar as other routes", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/")

    await expect(page.getByTestId("sidenav")).toBeVisible()
    await expect(page.getByTestId("generate-plan-default")).toBeVisible()
  })

  test("pressing 'd' toggles the root theme class and persists", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/settings")

    const initial = await page.evaluate(() =>
      document.documentElement.classList.contains("dark") ? "dark" : "light"
    )

    await page.keyboard.press("d")

    await expect
      .poll(() =>
        page.evaluate(() =>
          document.documentElement.classList.contains("dark") ? "dark" : "light"
        )
      )
      .not.toBe(initial)

    // Toggle back to restore state for other tests.
    await page.keyboard.press("d")
    await expect
      .poll(() =>
        page.evaluate(() =>
          document.documentElement.classList.contains("dark") ? "dark" : "light"
        )
      )
      .toBe(initial)
  })

  test("mobile viewport shows bottom nav and generate FAB", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto("/settings")

    const bottomNav = page.getByTestId("mobile-bottom-nav")
    await expect(bottomNav).toBeVisible()
    await expect(bottomNav.getByTestId("generate-plan-fab")).toBeVisible()

    // Desktop sidebar must be hidden on mobile (rendered but not visible).
    await expect(page.getByTestId("sidenav")).toBeHidden()
  })
})
