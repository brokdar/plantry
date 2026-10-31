import type { Locator } from "@playwright/test"

import { expect, mockAnchorToday, test } from "./helpers"

const DESTINATIONS = ["Today", "Plan", "Recipes", "Shop"]

async function expectFourDestinations(nav: Locator) {
  const links = nav.getByRole("link")
  await expect(links).toHaveText(DESTINATIONS)
  await expect(nav.getByRole("link", { name: "Today" })).toHaveAttribute(
    "href",
    "/today"
  )
  await expect(nav.getByRole("link", { name: "Plan" })).toHaveAttribute(
    "href",
    "/plan"
  )
  await expect(nav.getByRole("link", { name: "Recipes" })).toHaveAttribute(
    "href",
    "/components"
  )
  await expect(nav.getByRole("link", { name: "Shop" })).toHaveAttribute(
    "href",
    "/shop"
  )
}

test.describe("AppShell", () => {
  test("desktop rail shows the four destinations and settings at the bottom", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/settings")

    const sidebar = page.getByTestId("sidenav")
    await expect(sidebar).toBeVisible()
    await expect(sidebar.getByTestId("sidenav-brand")).toBeVisible()
    await expectFourDestinations(
      sidebar.getByRole("navigation", { name: "Primary" })
    )
    await expect(
      sidebar.getByRole("link", { name: "Settings" })
    ).toHaveAttribute("aria-current", "page")
    await expect(sidebar.getByTestId("generate-plan-default")).toBeVisible()

    // The phone gear in the Today header is hidden from 768 px up.
    await page.goto("/today")
    await expect(page.getByTestId("today-verdict")).toBeVisible()
    await expect(page.getByTestId("today-settings")).toBeHidden()
  })

  test("narrow rail (768 px) keeps labels and settings", async ({ page }) => {
    await page.setViewportSize({ width: 800, height: 900 })
    await page.goto("/")

    const sidebar = page.getByTestId("sidenav")
    await expect(sidebar).toHaveAttribute("data-collapsed", "true")
    await expectFourDestinations(
      sidebar.getByRole("navigation", { name: "Primary" })
    )
    await expect(sidebar.getByTestId("sidenav-settings")).toBeVisible()
  })

  test("Recipes stays active on ingredients and import", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    const recipes = page
      .getByTestId("sidenav")
      .getByRole("link", { name: "Recipes", exact: true })

    for (const path of ["/components", "/ingredients", "/import"]) {
      await page.goto(path)
      await expect(recipes).toHaveAttribute("aria-current", "page")
    }
  })

  test("the rail navigates to Today, Plan, Recipes and Shop", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/settings")
    const sidebar = page.getByTestId("sidenav")

    await sidebar.getByRole("link", { name: "Today" }).click()
    await expect(page).toHaveURL(/\/today$/)
    await expect(sidebar.getByRole("link", { name: "Today" })).toHaveAttribute(
      "aria-current",
      "page"
    )

    // exact: the Generate gate's "Add N more recipes" link also matches.
    await sidebar.getByRole("link", { name: "Recipes", exact: true }).click()
    await expect(page).toHaveURL(/\/components$/)
    await page.getByTestId("catalog-link-ingredients").click()
    await expect(page).toHaveURL(/\/ingredients$/)

    await sidebar.getByRole("link", { name: "Shop" }).click()
    await expect(page).toHaveURL(/\/shop$/)

    await sidebar.getByRole("link", { name: "Plan", exact: true }).click()
    await expect(page).toHaveURL(/\/plan$/)
  })

  test("/shop renders the shopping list for the current window", async ({
    page,
  }) => {
    await mockAnchorToday(page)
    await page.goto("/shop")

    // The title is the shopping range; without `?from&to` it is the server
    // default for the anchored window.
    await expect(page.getByRole("heading", { level: 1 })).toContainText(" to ")
    await expect(page.getByTestId("shop-count")).toBeVisible()
    await expect(page.getByRole("dialog")).toHaveCount(0)
    await page.unrouteAll({ behavior: "ignoreErrors" })
  })

  test("the palette keeps Ingredients and Import; Calendar and Archive are gone", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/components")
    const sidebar = page.getByTestId("sidenav")
    await expect(
      sidebar.getByRole("navigation", { name: "Primary" }).getByRole("link")
    ).toHaveCount(4)

    await page.keyboard.press("ControlOrMeta+k")
    const palette = page.getByRole("dialog")
    for (const name of ["Ingredients", "Import"]) {
      await expect(
        palette.getByRole("option", { name, exact: true })
      ).toBeVisible()
    }
    for (const name of ["Calendar", "Archive"]) {
      await expect(
        palette.getByRole("option", { name, exact: true })
      ).toHaveCount(0)
    }
  })

  // F5-AC5 (D2): removed routes render the not-found page, no redirects.
  test("/calendar, /day and /archive render the not-found page with a link to Today", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    for (const path of ["/calendar", "/day/2026-11-02", "/archive"]) {
      await page.goto(path)
      await expect(page.getByTestId("not-found")).toContainText(
        "Page not found."
      )
      await expect(page).toHaveURL(new RegExp(`${path}$`))
    }
    await page.getByRole("link", { name: "Go to Today" }).click()
    await expect(page).toHaveURL(/\/today$/)
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

  // F5-AC6 and D31: four tabs, the gear in the Today header, Generate in
  // the /plan header, and no top bar.
  test("phone shows four tabs, the gear in the Today header and Generate on Plan", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 })
    await page.goto("/today")

    const bottomNav = page.getByTestId("mobile-bottom-nav")
    await expect(bottomNav).toBeVisible()
    await expectFourDestinations(bottomNav)
    await expect(
      bottomNav.getByRole("link", { name: "Today" })
    ).toHaveAttribute("aria-current", "page")
    await expect(bottomNav.getByRole("link")).toHaveCount(4)
    await expect(bottomNav.getByRole("button")).toHaveCount(0)
    await expect(page.getByTestId("topbar")).toHaveCount(0)

    // Desktop sidebar must be hidden on mobile (rendered but not visible).
    await expect(page.getByTestId("sidenav")).toBeHidden()

    await bottomNav.getByRole("link", { name: "Plan" }).click()
    await expect(page).toHaveURL(/\/plan$/)
    await expect(page.getByTestId("generate-plan-compact")).toBeVisible()

    await bottomNav.getByRole("link", { name: "Today" }).click()
    const gear = page.getByTestId("today-settings")
    await expect(gear).toBeVisible()
    await gear.click()
    await expect(page).toHaveURL(/\/settings/)
  })
})
