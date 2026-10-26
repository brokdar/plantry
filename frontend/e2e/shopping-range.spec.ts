import type { Page } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  mockAnchorToday,
  seedComposedWithStub,
  seedSlot,
  test,
  uid,
} from "./helpers"

/** Seed a plate via the date-keyed POST /api/plates endpoint. */
async function seedPlateByDate(
  date: string,
  slotId: number,
  foodId: number
): Promise<{ id: number }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId },
  })
  const plate = (await res.json()) as { id: number }
  await ctx.post(`/api/plates/${plate.id}/components`, {
    data: { food_id: foodId, portions: 1 },
  })
  await ctx.dispose()
  return plate
}

async function deletePlate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/plates/${id}`)
  await ctx.dispose()
}

function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function dateOffset(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

/**
 * A ?date= that makes the planner (anchor "today") view the previous window,
 * today-7..today-1. The route floors the day diff (target midnight vs. now) to
 * a multiple of 7: today-6 lands on -7 at any time of day, whereas today-7 can
 * round to -8 and land on -14.
 */
const previousWindowDate = () => dateOffset(-6)

/** "Oct 26 — Nov 1" for today+fromOffset … today+toOffset (en-US locale). */
function rangeLabel(fromOffset: number, toOffset: number): string {
  const fmt = new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
  })
  const at = (offset: number) => {
    const d = new Date()
    d.setDate(d.getDate() + offset)
    return fmt.format(d)
  }
  return `${at(fromOffset)} — ${at(toOffset)}`
}

function waitForShoppingList(page: Page) {
  return page.waitForResponse(
    (r) =>
      r.url().includes("/api/shopping-list") && r.request().method() === "GET"
  )
}

test.describe("Shopping panel — range + presets + purchased state", () => {
  test.beforeEach(async ({ page }) => {
    await mockAnchorToday(page)
  })

  test("opens and renders shopping list for active window", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.shop_open_${tag}`, "Moon", 940)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Pasta ${tag}`, role: "main" },
      tag
    )

    const today = todayISO()
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(today, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")

      await page.getByRole("button", { name: /shopping/i }).click()
      const dialog = page.getByRole("dialog")
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // Food seeded for today should appear in the list
      await expect(dialog.getByText(new RegExp(`Stub.*${tag}`))).toBeVisible()
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
      await page.evaluate(() => localStorage.clear())
    }
  })

  test("'Next window' preset chip updates the range description", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.shop_preset_${tag}`, "Sun", 939)

    try {
      // Anchor "today" without ?date= → the viewed window is today..today+6.
      // Today is the window start, so "This window" is the default.
      await page.goto("/")

      await page.getByRole("button", { name: /shopping/i }).click()
      const dialog = page.getByRole("dialog")
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      const thisWindow = dialog.getByRole("button", { name: "This window" })
      const nextWindow = dialog.getByRole("button", { name: "Next window" })
      await expect(thisWindow).toHaveAttribute("aria-pressed", "true")
      await expect(nextWindow).toHaveAttribute("aria-pressed", "false")
      await expect(dialog.getByText(rangeLabel(0, 6))).toBeVisible()

      await nextWindow.click()

      await expect(nextWindow).toHaveAttribute("aria-pressed", "true")
      await expect(thisWindow).toHaveAttribute("aria-pressed", "false")
      await expect(dialog.getByText(rangeLabel(7, 13))).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
      try {
        await page.evaluate(() => localStorage.clear())
      } catch {
        // best-effort: page may already be closed
      }
    }
  })

  test("purchased item persists across panel close/reopen (localStorage)", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.shop_persist_${tag}`, "Moon", 938)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Lentils ${tag}`, role: "main" },
      tag
    )

    const today = todayISO()
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(today, slot.id, food.id)
      plateId = plate.id

      // View the previous window (today-7..today-1): today lies outside it, so
      // "Next window" (today..today+6) is deterministic and its cache is cold.
      await page.goto(`/?date=${previousWindowDate()}`)

      await page.getByRole("button", { name: /shopping/i }).click()
      const dialog = page.getByRole("dialog")
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // Register waitForResponse BEFORE clicking so we don't miss it.
      const chip = dialog.getByRole("button", { name: "Next window" })
      const shoppingListResp = waitForShoppingList(page)
      await chip.click()
      await shoppingListResp
      await expect(chip).toHaveAttribute("aria-pressed", "true")

      // Check the ingredient
      const checkbox = dialog
        .getByRole("checkbox", { name: new RegExp(`Stub.*${tag}`, "i") })
        .first()
      await expect(checkbox).toBeVisible()
      await checkbox.click()
      await expect(checkbox).toBeChecked()

      // Close and reopen
      await page.keyboard.press("Escape")
      await page.getByRole("button", { name: /shopping/i }).click()
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      // Chip state is restored from the still-mounted ShoppingPanel
      await expect(chip).toHaveAttribute("aria-pressed", "true")

      // Item should still be checked
      await expect(
        dialog
          .getByRole("checkbox", { name: new RegExp(`Stub.*${tag}`, "i") })
          .first()
      ).toBeChecked()
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
      try {
        await page.evaluate(() => localStorage.clear())
      } catch {
        // best-effort: page may already be closed
      }
    }
  })

  test("purchased state is keyed per range — switching presets does not contaminate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.shop_key_${tag}`, "Moon", 937)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Quinoa ${tag}`, role: "main" },
      tag
    )

    const plateIds: number[] = []

    try {
      // One plate in each range so the item is listed in both:
      // yesterday → "This window" (today-7..today-1),
      // today → "Next window" (today..today+6).
      plateIds.push(
        (await seedPlateByDate(dateOffset(-1), slot.id, food.id)).id
      )
      plateIds.push((await seedPlateByDate(todayISO(), slot.id, food.id)).id)

      await page.goto(`/?date=${previousWindowDate()}`)

      await page.getByRole("button", { name: /shopping/i }).click()
      const dialog = page.getByRole("dialog")
      await expect(
        dialog.getByRole("heading", { name: /shopping list/i })
      ).toBeVisible()

      const item = dialog
        .getByRole("checkbox", { name: new RegExp(`Stub.*${tag}`, "i") })
        .first()

      // --- Range A: Next window (register waitForResponse BEFORE click) ---
      const chipA = dialog.getByRole("button", { name: "Next window" })
      const resp = waitForShoppingList(page)
      await chipA.click()
      await resp
      await expect(chipA).toHaveAttribute("aria-pressed", "true")

      // Mark item purchased in Range A
      await expect(item).toBeVisible()
      await item.click()
      await expect(item).toBeChecked()

      // --- Range B: This window (different localStorage key) ---
      const chipB = dialog.getByRole("button", { name: "This window" })
      await chipB.click()
      await expect(chipB).toHaveAttribute("aria-pressed", "true")

      // Range B has its own purchased state: listed, but not checked
      await expect(item).toBeVisible()
      await expect(item).not.toBeChecked()

      // --- Switch back to Range A ---
      await chipA.click()
      await expect(chipA).toHaveAttribute("aria-pressed", "true")

      // Range A purchased state still intact
      await expect(item).toBeChecked()
    } finally {
      for (const id of plateIds) await deletePlate(id)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
      try {
        await page.evaluate(() => localStorage.clear())
      } catch {
        // best-effort: page may already be closed
      }
    }
  })
})
