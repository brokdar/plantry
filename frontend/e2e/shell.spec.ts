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
      /planner/i,
      /components/i,
      /ingredients/i,
      /templates/i,
      /import/i,
      /archive/i,
      /settings/i,
    ]) {
      await expect(sidebar.getByRole("link", { name: label })).toBeVisible()
    }

    await expect(sidebar.getByTestId("generate-plan-default")).toBeVisible()
  })

  test("planner route uses the icon-rail sidebar variant", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/")

    await expect(page.getByTestId("sidenav-rail")).toBeVisible()
    await expect(page.getByTestId("sidenav")).toBeHidden()
    await expect(page.getByTestId("generate-plan-rail")).toBeVisible()
  })

  test("theme toggle flips the root class and persists", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/settings")

    const initial = await page.evaluate(() =>
      document.documentElement.classList.contains("dark") ? "dark" : "light"
    )

    await page.getByTestId("theme-toggle").click()

    const next = await page.evaluate(() =>
      document.documentElement.classList.contains("dark") ? "dark" : "light"
    )

    expect(next).not.toBe(initial)

    // Toggle back to restore state for other tests.
    await page.getByTestId("theme-toggle").click()
    const restored = await page.evaluate(() =>
      document.documentElement.classList.contains("dark") ? "dark" : "light"
    )
    expect(restored).toBe(initial)
  })

  test("mobile viewport shows bottom nav and generate FAB", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto("/settings")

    const bottomNav = page.getByTestId("mobile-bottom-nav")
    await expect(bottomNav).toBeVisible()
    await expect(bottomNav.getByTestId("generate-plan-fab")).toBeVisible()

    // Desktop sidebars must be hidden on mobile (rendered but not visible).
    await expect(page.getByTestId("sidenav")).toBeHidden()
    await expect(page.getByTestId("sidenav-rail")).toBeHidden()
  })
})
