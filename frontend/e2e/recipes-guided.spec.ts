import type { Page } from "@playwright/test"

import { expect, test } from "./helpers"

// D22: the shared e2e DB accumulates recipes, so the guided state and the
// Generate gate are driven by a stubbed GET /api/setup, never the real count.
async function stubRecipeCount(page: Page, recipe_count: number) {
  await page.route("**/api/setup", (route) =>
    route.fulfill({
      json: { completed: true, recipe_count, recipe_target: 10 },
    })
  )
}

test.describe("Guided Recipes state", () => {
  test("below 10 recipes: progress, both cards and a gated Generate button", async ({
    page,
  }) => {
    await stubRecipeCount(page, 3)
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/components")

    const guide = page.getByTestId("recipe-onboarding")
    await expect(
      guide.getByText(
        "3 of 10 recipes. Plantry needs 10 to plan a varied week."
      )
    ).toBeVisible()
    await expect(guide.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "3"
    )
    await expect(
      guide.getByRole("heading", { name: "Import from a link" })
    ).toBeVisible()
    await expect(
      guide.getByRole("heading", { name: "Create manually" })
    ).toBeVisible()

    const sidebar = page.getByTestId("sidenav")
    const generate = sidebar.getByTestId("generate-plan-default")
    await expect(generate).toHaveAttribute("aria-disabled", "true")
    await expect(generate).toHaveAccessibleDescription(
      "Add 7 more recipes to start planning"
    )
    await expect(
      sidebar.getByRole("link", {
        name: "Add 7 more recipes to start planning",
      })
    ).toHaveAttribute("href", "/components")

    // aria-disabled stays focusable, and focus explains the gate.
    await generate.focus()
    await expect(page.getByRole("tooltip")).toContainText(
      "Add 7 more recipes to start planning"
    )
    // Playwright treats aria-disabled as not enabled, hence force.
    await generate.click({ force: true })
    await expect(page).toHaveURL(/\/components$/)
    await expect(page.getByTestId("chat-composer-input")).toHaveCount(0)
  })

  test("the import card navigates to /import", async ({ page }) => {
    await stubRecipeCount(page, 3)
    await page.goto("/components")

    await page.getByTestId("onboarding-import").click()
    await expect(page).toHaveURL(/\/import$/)
  })

  test("at 10 recipes: no cards and an enabled Generate button", async ({
    page,
  }) => {
    await stubRecipeCount(page, 10)
    await page.setViewportSize({ width: 1440, height: 900 })
    const setupLoaded = page.waitForResponse("**/api/setup")
    await page.goto("/components")
    await setupLoaded

    await expect(page.getByTestId("component-sort")).toBeVisible()
    await expect(page.getByTestId("recipe-onboarding")).toHaveCount(0)
    const generate = page
      .getByTestId("sidenav")
      .getByTestId("generate-plan-default")
    await expect(generate).toBeVisible()
    await expect(generate).not.toHaveAttribute("aria-disabled")
    await expect(page.getByTestId("generate-plan-locked-link")).toHaveCount(0)
  })

  test("reaching 10 after the guide collapses it with a one-time toast", async ({
    page,
  }) => {
    await stubRecipeCount(page, 9)
    await page.goto("/components")
    await expect(page.getByTestId("recipe-onboarding")).toBeVisible()

    await page.unroute("**/api/setup")
    await stubRecipeCount(page, 10)
    await page.reload()

    await expect(
      page.getByText("10 recipes. You're ready to plan your first week.")
    ).toBeVisible()
    await expect(page.getByTestId("recipe-onboarding")).toHaveCount(0)

    // The flag is spent: a later visit stays quiet.
    const setupLoaded = page.waitForResponse("**/api/setup")
    await page.reload()
    await setupLoaded
    await expect(page.getByTestId("component-sort")).toBeVisible()
    await expect(
      page.getByText("10 recipes. You're ready to plan your first week.")
    ).toHaveCount(0)
  })

  test("phone: the compact plan-header button is gated too", async ({
    page,
  }) => {
    await stubRecipeCount(page, 3)
    await page.setViewportSize({ width: 375, height: 812 })
    await page.goto("/plan")

    const generate = page.getByTestId("generate-plan-compact")
    await expect(generate).toHaveAttribute("aria-disabled", "true")
    // A tap (no hover on phones) shows the explanation.
    await generate.click({ force: true })
    await expect(page.getByRole("tooltip")).toContainText(
      "Add 7 more recipes to start planning"
    )
  })
})
