import type { Locator } from "@playwright/test"

import { expect, mockAnchorToday, test } from "./helpers"

const DESTINATIONS = ["Today", "Plan", "Recipes", "Shop"]

function localToday() {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

async function expectFourDestinations(nav: Locator) {
  const links = nav.getByRole("link")
  await expect(links).toHaveText(DESTINATIONS)
  await expect(nav.getByRole("link", { name: "Today" })).toHaveAttribute(
    "href",
    `/day/${localToday()}`
  )
  await expect(nav.getByRole("link", { name: "Plan" })).toHaveAttribute(
    "href",
    "/"
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

    // The phone gear is hidden from 768 px up.
    await expect(page.getByTestId("topbar-settings")).toBeHidden()
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
    await expect(page).toHaveURL(new RegExp(`/day/${localToday()}`))

    // exact: the Generate gate's "Add N more recipes" link also matches.
    await sidebar.getByRole("link", { name: "Recipes", exact: true }).click()
    await expect(page).toHaveURL(/\/components$/)
    await page.getByTestId("catalog-link-ingredients").click()
    await expect(page).toHaveURL(/\/ingredients$/)

    await sidebar.getByRole("link", { name: "Shop" }).click()
    await expect(page).toHaveURL(/\/shop$/)

    await sidebar.getByRole("link", { name: "Plan", exact: true }).click()
    await expect(page).toHaveURL(/\/$/)
  })

  test("/shop renders the shopping list for the current window", async ({
    page,
  }) => {
    await mockAnchorToday(page)
    await page.goto("/shop")

    await expect(
      page.getByRole("heading", { level: 1, name: "Shopping list" })
    ).toBeVisible()
    await expect(page.getByRole("dialog")).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "This window" })
    ).toHaveAttribute("aria-pressed", "true")
    await page.unrouteAll({ behavior: "ignoreErrors" })
  })

  test("calendar, presets and archive stay reachable via the command palette", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/components")
    const sidebar = page.getByTestId("sidenav")
    await expect(
      sidebar.getByRole("navigation", { name: "Primary" }).getByRole("link")
    ).toHaveCount(4)
    await expect(sidebar.getByRole("link", { name: "Calendar" })).toHaveCount(0)
    await expect(page.getByTestId("catalog-secondary-actions")).toBeVisible()

    await page.keyboard.press("ControlOrMeta+k")
    const palette = page.getByRole("dialog")
    for (const name of ["Calendar", "Archive", "Ingredients", "Import"]) {
      await expect(
        palette.getByRole("option", { name, exact: true })
      ).toBeVisible()
    }
    await expect(
      palette.getByRole("option", { name: "Open Presets library" })
    ).toBeVisible()

    await palette.getByRole("option", { name: "Calendar", exact: true }).click()
    await expect(page).toHaveURL(/\/calendar/)
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

  test("phone shows the four tabs, a compact generate button and a settings gear", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 })
    await page.goto("/ingredients")

    const bottomNav = page.getByTestId("mobile-bottom-nav")
    await expect(bottomNav).toBeVisible()
    await expectFourDestinations(bottomNav)
    await expect(
      bottomNav.getByRole("link", { name: "Recipes" })
    ).toHaveAttribute("aria-current", "page")
    // UX §2.1: exactly four tab items; Generate lives in the top bar.
    await expect(bottomNav.getByRole("link")).toHaveCount(4)
    await expect(bottomNav.getByRole("button")).toHaveCount(0)
    await expect(
      page.getByTestId("topbar").getByTestId("generate-plan-compact")
    ).toBeVisible()

    // Desktop sidebar must be hidden on mobile (rendered but not visible).
    await expect(page.getByTestId("sidenav")).toBeHidden()

    const gear = page.getByTestId("topbar-settings")
    await expect(gear).toBeVisible()
    await gear.click()
    await expect(page).toHaveURL(/\/settings/)
  })
})
