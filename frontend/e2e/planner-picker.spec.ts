import { expect, test } from "./helpers"

import {
  cleanupFood,
  cleanupSlot,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  uid,
} from "./helpers"

// Composed foods now require at least one child. Helper that wraps
// seedComposedFood with a throwaway leaf so tests don't need to set up the
// child ladder themselves.
async function seedComposedWithStub(
  data: Parameters<typeof seedComposedFood>[0],
  tag: string
) {
  const stub = await seedLeafFood({ name: `Stub ${tag}-${data.name}` })
  const composed = await seedComposedFood({
    ...data,
    children: data.children ?? [
      {
        child_id: stub.id,
        amount: 100,
        unit: "g",
        grams: 100,
        sort_order: 0,
      },
    ],
  })
  return { composed, stub }
}

test.describe("Planner picker route", () => {
  test("empty cell navigates to picker, tray accumulates, Save creates plate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 996)
    const { composed: main, stub: mainStub } = await seedComposedWithStub(
      { name: `Sushi ${tag}`, role: "main" },
      tag
    )
    const { composed: side, stub: sideStub } = await seedComposedWithStub(
      { name: `Miso ${tag}`, role: "side_veg" },
      tag
    )

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
      await cleanupFood(side.id)
      await cleanupFood(main.id)
      await cleanupFood(mainStub.id)
      await cleanupFood(sideStub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("favorites prefilter narrows catalog", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 995)
    const { composed: fav, stub: favStub } = await seedComposedWithStub(
      { name: `Tacos ${tag}`, role: "main" },
      tag
    )
    const { composed: other, stub: otherStub } = await seedComposedWithStub(
      { name: `Lasagna ${tag}`, role: "main" },
      tag
    )

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await cell.getByRole("button", { name: /plan meal/i }).click()
      await expect(page.getByTestId("picker-target")).toBeVisible()

      // Mark Tacos as favorite via its card heart.
      const favResp = page.waitForResponse(
        (r) =>
          /\/foods\/\d+\/favorite$/.test(r.url()) &&
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
      await cleanupFood(other.id)
      await cleanupFood(fav.id)
      await cleanupFood(favStub.id)
      await cleanupFood(otherStub.id)
      await cleanupSlot(slot.id)
    }
  })
})
