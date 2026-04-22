import { expect, test } from "./helpers"

import {
  cleanupComponent,
  cleanupSlot,
  seedComponent,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Planner picker route", () => {
  test("empty cell navigates to picker, tray accumulates, Save creates plate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 996)
    const main = await seedComponent({
      name: `Sushi ${tag}`,
      role: "main",
    })
    const side = await seedComponent({
      name: `Miso ${tag}`,
      role: "side_veg",
    })

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()

      // Empty cell navigates to the picker route.
      await cell.getByRole("button", { name: /plan meal/i }).click()
      await expect(page).toHaveURL(
        new RegExp(`/planner/\\d+/0/${slot.id}/pick`)
      )

      // Target context strip renders.
      await expect(page.getByTestId("picker-target")).toBeVisible()

      // Pick main → tray shows 1 component.
      await page.getByTestId(`picker-card-${main.id}`).click()
      await expect(page.getByTestId(`tray-item-${main.id}`)).toBeVisible()

      // Pick side → tray shows 2.
      await page.getByTestId(`picker-card-${side.id}`).click()
      await expect(page.getByTestId(`tray-item-${side.id}`)).toBeVisible()

      // Save creates the plate and returns to planner.
      const createResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await page.getByTestId("tray-save").click()
      await createResp

      await expect(page).toHaveURL(/\/$/)
      await expect(cell.getByText(`Sushi ${tag}`)).toBeVisible()
    } finally {
      await cleanupComponent(side.id)
      await cleanupComponent(main.id)
      await cleanupSlot(slot.id)
    }
  })

  test("favorites prefilter narrows catalog", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 995)
    const fav = await seedComponent({ name: `Tacos ${tag}`, role: "main" })
    const other = await seedComponent({ name: `Lasagna ${tag}`, role: "main" })

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await cell.getByRole("button", { name: /plan meal/i }).click()
      await expect(page.getByTestId("picker-target")).toBeVisible()

      // Mark Tacos as favorite via its card heart.
      const favResp = page.waitForResponse(
        (r) =>
          /\/components\/\d+\/favorite$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await page
        .getByTestId(`picker-card-${fav.id}`)
        .getByRole("button", { name: /favorite/i })
        .click()
      await favResp

      // Activate Favorites prefilter.
      await page.getByTestId("picker-filter-favorites").click()

      await expect(page.getByTestId(`picker-card-${fav.id}`)).toBeVisible()
      await expect(page.getByTestId(`picker-card-${other.id}`)).toHaveCount(0)
    } finally {
      await cleanupComponent(other.id)
      await cleanupComponent(fav.id)
      await cleanupSlot(slot.id)
    }
  })
})
