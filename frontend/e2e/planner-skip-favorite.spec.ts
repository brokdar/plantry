import { expect, test } from "./helpers"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  uid,
} from "./helpers"

test.describe("Slot skip + favorite (redesign)", () => {
  test("toggle favorite from slot card persists on the component", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Sun", 998)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const main = await seedComposedFood({
      name: `Ramen ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)

      // Seed a plate by clicking the empty cell → picker.
      const createResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /plan meal/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Ramen ${tag}`) })
        .click()
      await page.getByTestId("tray-save").click()
      await createResp
      await expect(cell.getByText(`Ramen ${tag}`)).toBeVisible()

      // Hover reveals the favorite button; clicking flips it to pressed state.
      await cell.hover()
      const favBtn = cell.getByTestId("slot-action-favorite")
      const favResp = page.waitForResponse(
        (r) =>
          /\/foods\/\d+\/favorite$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await favBtn.click()
      await favResp

      // Verify backend flipped the flag.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const detail = await ctx.get(`/api/foods/${main.id}`)
      expect(detail.ok()).toBeTruthy()
      const body = await detail.json()
      expect(body.favorite).toBe(true)
    } finally {
      await cleanupFood(main.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("toggle skip state via slot menu — hatch appears, components clear", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 997)
    const stub = await seedLeafFood({ name: `Stub ${tag}` })
    const main = await seedComposedFood({
      name: `Pho ${tag}`,
      role: "main",
      children: [
        {
          child_id: stub.id,
          amount: 100,
          unit: "g",
          grams: 100,
          sort_order: 0,
        },
      ],
    })

    try {
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)

      // Seed a plate to have a skip target.
      const createResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /plan meal/i }).click()
      await page.getByRole("button", { name: new RegExp(`Pho ${tag}`) }).click()
      await page.getByTestId("tray-save").click()
      await createResp
      await expect(cell.getByText(`Pho ${tag}`)).toBeVisible()

      // Open the per-plate dropdown menu and click "Mark as skip".
      const skipResp = page.waitForResponse(
        (r) =>
          /\/plates\/\d+\/skip$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await cell.hover()
      await cell.getByRole("button", { name: /actions/i }).click()
      await page.getByRole("menuitem", { name: /mark as skip/i }).click()
      await skipResp

      // Skipped cell exposes the SKIP label and no longer shows component name.
      await expect(cell.locator('[data-slot-state="skipped"]')).toBeVisible()
      await expect(cell.getByText(/^skip$/i)).toBeVisible()
      await expect(cell.getByText(`Pho ${tag}`)).toHaveCount(0)
    } finally {
      await cleanupFood(main.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
