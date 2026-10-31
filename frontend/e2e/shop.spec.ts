import type { Page } from "@playwright/test"

import {
  API,
  addDays,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// F7: /shop (UX §5.7). Each test cooks its own AC-1 Chili batch (6 portions
// of a recipe for 4 with 400 g kidney beans and 2 onions) in its own slot
// and future window, so its leaves and amounts belong to it alone. The tag
// leads the names: the store-section classifier weighs the last words.

interface Setup {
  start: string
  end: string
  cookDate: string
  chili: string
  beans: string
  onion: string
  cleanup: () => Promise<void>
}

async function setup(): Promise<Setup> {
  const tag = uid()
  const { start, days } = await futureWindow()
  const slot = await seedSlot(`slot.shop-${tag}`, "Moon", 1_000_000)
  const beans = await seedLeafFood({ name: `${tag} Kidney beans (canned)` })
  const onion = await seedLeafFood({ name: `${tag} Onion` })
  const chili = await seedComposedFood({
    name: `Chili ${tag}`,
    role: "main",
    reference_portions: 4,
    children: [
      { child_id: beans.id, amount: 400, unit: "g", grams: 400, sort_order: 0 },
      {
        child_id: onion.id,
        amount: 2,
        unit: "piece",
        grams: 240,
        sort_order: 1,
      },
    ],
  })

  const ctx = await apiRequest.newContext({ baseURL: API })
  const cookDate = days[1]
  const plate = await ctx.post("/api/plates", {
    data: {
      date: cookDate,
      slot_id: slot.id,
      components: [{ food_id: chili.id, portions: 1 }],
    },
  })
  expect(plate.ok(), await plate.text()).toBeTruthy()
  const plan = await ctx.get(`/api/plan?start=${start}`)
  const { sessions } = (await plan.json()) as {
    sessions: { id: number; food_id: number }[]
  }
  const session = sessions.find((s) => s.food_id === chili.id)
  expect(session, "the fresh Chili session").toBeTruthy()
  // A batch of 6 whatever the household's eaters (D11: explicit edits win).
  const resized = await ctx.put(`/api/cook-sessions/${session!.id}`, {
    data: { portions: 6, frozen_portions: 0, note: null },
  })
  expect(resized.ok(), await resized.text()).toBeTruthy()
  await ctx.dispose()

  return {
    start,
    end: days[6],
    cookDate,
    chili: chili.name,
    beans: beans.name,
    onion: onion.name,
    cleanup: async () => {
      await cleanupSlot(slot.id)
      await cleanupFood(chili.id)
      await cleanupFood(beans.id)
      await cleanupFood(onion.id)
    },
  }
}

function section(page: Page, name: string) {
  return page.getByRole("region", { name, exact: true })
}

function row(page: Page, name: string) {
  return page.getByTestId("shop-row").filter({ hasText: name })
}

function weekdayShort(iso: string) {
  return new Intl.DateTimeFormat("en-US", { weekday: "short" }).format(
    new Date(`${iso}T00:00:00`)
  )
}

async function openShop(page: Page, from: string, to: string) {
  const list = page.waitForResponse(
    (r) => r.url().includes("/api/shopping-list?") && r.ok()
  )
  await page.goto(`/shop?from=${from}&to=${to}`)
  await list
}

test.describe("Shop", () => {
  // F7-AC1 (= AC-1 step 2), F7-AC4
  test("lists the batch's beans and onions with the badge; ticks persist per range", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await openShop(page, s.start, s.end)

      const onion = row(page, s.onion)
      await expect(section(page, "Produce").getByText(s.onion)).toBeVisible()
      await expect(onion.getByTestId("shop-quantity")).toHaveText("3 pcs")
      const beans = row(page, s.beans)
      await expect(
        section(page, "Cans & jars").getByText(s.beans)
      ).toBeVisible()
      await expect(beans.getByTestId("shop-quantity")).toHaveText("600 g")

      await beans
        .getByRole("button", { name: `Where ${s.beans} is used` })
        .click()
      const use = beans.getByTestId("shop-used-in")
      await expect(use).toContainText(
        `${s.chili} · ${weekdayShort(s.cookDate)}`
      )
      await expect(use.locator("[data-batch-badge]")).toHaveText("Cook × 6")

      // Ticking: the count follows, the row sinks and survives a reload.
      const count = page.getByTestId("shop-count")
      await expect(count).toContainText("0 ticked")
      await onion.getByRole("checkbox").check()
      await expect(count).toContainText("1 ticked")
      await expect(onion).toHaveAttribute("data-ticked", "true")
      const key = `plantry:purchased:range:${s.start}:${s.end}`
      const stored = await page.evaluate((k) => localStorage.getItem(k), key)
      expect(stored).toMatch(/^\[\d+\]$/)

      await openShop(page, s.start, s.end)
      await expect(row(page, s.onion).getByRole("checkbox")).toBeChecked()
      await expect(count).toContainText("1 ticked")

      // Another range has its own ticks.
      await openShop(page, addDays(s.start, 1), s.end)
      await expect(row(page, s.onion).getByRole("checkbox")).not.toBeChecked()
    } finally {
      await s.cleanup()
    }
  })

  // F7-AC3
  test("range presets follow the viewed window; Next window shifts by 7 days", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await openShop(page, addDays(s.start, 2), s.end)
      await page.getByTestId("shop-range").click()
      await expect(
        page.getByRole("button", { name: /This window/ })
      ).toHaveAttribute("aria-pressed", "false")

      const next = page.waitForResponse(
        (r) =>
          r.url().includes(`from=${addDays(s.start, 7)}`) &&
          r.url().includes(`to=${addDays(s.end, 7)}`) &&
          r.ok()
      )
      await page.getByRole("button", { name: /Next window/ }).click()
      await next
      await expect(page).toHaveURL(
        `/shop?from=${addDays(s.start, 7)}&to=${addDays(s.end, 7)}`
      )
      await expect(row(page, s.onion)).toHaveCount(0)

      // Back: "This window" of the next window's window is that window.
      await page.getByTestId("shop-range").click()
      await expect(
        page.getByRole("button", { name: /This window/ })
      ).toHaveAttribute("aria-pressed", "true")
      await page.getByRole("button", { name: /Next window/ }).click()
      await expect(page).toHaveURL(
        `/shop?from=${addDays(s.start, 14)}&to=${addDays(s.end, 14)}`
      )

      // A custom range.
      await page.getByTestId("shop-range").click()
      await page.getByLabel("From").fill(s.start)
      await page.getByLabel("To").fill(s.end)
      await page.getByRole("button", { name: "Show list" }).click()
      await expect(page).toHaveURL(`/shop?from=${s.start}&to=${s.end}`)
      await expect(row(page, s.onion)).toBeVisible()
    } finally {
      await s.cleanup()
    }
  })

  // F7-AC2
  test("moving an item to Frozen re-renders it there and persists across reload", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await openShop(page, s.start, s.end)
      await expect(
        section(page, "Cans & jars").getByText(s.beans)
      ).toBeVisible()

      const moved = page.waitForResponse(
        (r) =>
          r.url().includes("/shop-section") &&
          r.request().method() === "PUT" &&
          r.ok()
      )
      const refetch = page.waitForResponse(
        (r) => r.url().includes("/api/shopping-list?") && r.ok()
      )
      await row(page, s.beans)
        .getByRole("button", { name: `Options for ${s.beans}` })
        .click()
      await page.getByRole("menuitemradio", { name: "Frozen" }).click()
      await moved
      await refetch
      await expect(section(page, "Frozen").getByText(s.beans)).toBeVisible()
      await expect(section(page, "Cans & jars").getByText(s.beans)).toHaveCount(
        0
      )

      await openShop(page, s.start, s.end)
      await expect(section(page, "Frozen").getByText(s.beans)).toBeVisible()

      // The user's choice can be undone: back to the classifier.
      await row(page, s.beans)
        .getByRole("button", { name: `Options for ${s.beans}` })
        .click()
      const reset = page.waitForResponse(
        (r) => r.url().includes("/shop-section") && r.ok()
      )
      await page.getByRole("menuitem", { name: "Reset to automatic" }).click()
      await reset
      await expect(
        section(page, "Cans & jars").getByText(s.beans)
      ).toBeVisible()
    } finally {
      await s.cleanup()
    }
  })

  test("an unplanned range says to plan it and links to the plan", async ({
    page,
  }) => {
    const { start, days } = await futureWindow()
    await openShop(page, start, days[6])
    // The worker owns this window (futureWindow), so it has no plates.
    const empty = page.getByTestId("shop-empty")
    await expect(empty).toContainText("Plan this window to see what to buy")
    await empty.getByRole("link", { name: "Go to plan" }).click()
    await expect(page).toHaveURL(`/plan?start=${start}`)
  })
})
