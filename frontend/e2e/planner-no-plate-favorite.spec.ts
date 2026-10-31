import type { Locator, Page } from "@playwright/test"

import { expect, test } from "./helpers"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  mockAnchorToday,
  pickAndCommitFood,
  seedComposedFood,
  seedComposedWithStub,
  seedLeafFood,
  seedPlateWithComponent,
  seedSlot,
  uid,
} from "./helpers"

function dateOffset(offset: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offset)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

async function setOverride(date: string, slotId: number, mode: string) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.put(`/api/availability/${date}/${slotId}`, {
    data: { mode, remove_plate: true },
  })
  expect(res.ok(), `override ${date}/${slotId}: ${res.status()}`).toBeTruthy()
  await ctx.dispose()
}

async function clearOverride(date: string, slotId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/availability/${date}/${slotId}`)
  await ctx.dispose()
}

function isOverridePut(url: string, method: string, slotId: number) {
  return (
    method === "PUT" &&
    new RegExp(`/api/availability/\\d{4}-\\d{2}-\\d{2}/${slotId}$`).test(url)
  )
}

async function markViaMenu(page: Page, cell: Locator, slotId: number) {
  const putResp = page.waitForResponse((r) =>
    isOverridePut(r.url(), r.request().method(), slotId)
  )
  await cell.hover()
  await cell.getByRole("button", { name: /actions/i }).click()
  await page.getByRole("menuitem", { name: "Mark as No plate" }).click()
  const res = await putResp
  expect(res.ok()).toBeTruthy()
}

async function centerOf(loc: Locator): Promise<{ x: number; y: number }> {
  const box = await loc.boundingBox()
  if (!box) throw new Error("Locator has no bounding box")
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}

test.describe("Planner — No plate + favorite", () => {
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

      // Seed a plate via the picker sheet.
      await cell.getByRole("button", { name: /plan meal/i }).click()
      const sheet1 = page.getByRole("dialog")
      await expect(sheet1).toBeVisible()
      await sheet1.locator("input").first().fill(`Ramen ${tag}`)
      await pickAndCommitFood(page, sheet1, new RegExp(`Ramen ${tag}`))
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

  test("mark a planned cell No plate, undo it, mark again, then plan the meal", async ({
    page,
  }) => {
    await mockAnchorToday(page)
    const tag = uid()
    const slot = await seedSlot(`slot.noplate_${tag}`, "Moon", 997)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Pho ${tag}`, role: "main" },
      tag
    )
    const day0 = dateOffset(0)

    try {
      await seedPlateWithComponent(day0, slot.id, {
        food_id: food.id,
        portions: 1,
      })
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell.getByText(`Pho ${tag}`)).toBeVisible()

      // Mark → the plate is gone and the cell says No plate.
      await markViaMenu(page, cell, slot.id)
      await expect(cell.locator('[data-slot-state="none"]')).toBeVisible()
      await expect(cell.getByText("No plate")).toBeVisible()
      await expect(cell.getByText(`Pho ${tag}`)).toHaveCount(0)

      // Undo → the override is cleared and the plate comes back.
      const recreated = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/plan/restore") &&
          r.request().method() === "POST"
      )
      await page.getByRole("button", { name: "Undo", exact: true }).click()
      await recreated
      await expect(cell.getByText(`Pho ${tag}`)).toBeVisible()
      await expect(cell.locator('[data-slot-state="none"]')).toHaveCount(0)

      // Mark again; the state survives a reload.
      await markViaMenu(page, cell, slot.id)
      await expect(cell.locator('[data-slot-state="none"]')).toBeVisible()
      await page.reload()
      await expect(cell.locator('[data-slot-state="none"]')).toBeVisible()
      await expect(cell.getByText(`Pho ${tag}`)).toHaveCount(0)

      // "Plan this meal" drops the none override: the plan default applies.
      const planned = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/api/availability/${day0}/${slot.id}`) &&
          r.request().method() === "DELETE"
      )
      await cell.getByRole("button", { name: "Plan this meal" }).click()
      expect((await planned).ok()).toBeTruthy()
      await expect(cell.getByTestId("slot-empty-add")).toBeVisible()
      await expect(cell.locator('[data-slot-state="none"]')).toHaveCount(0)
    } finally {
      await clearOverride(day0, slot.id)
      await cleanupSlot(slot.id)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
    }
  })

  test("a plate cannot be dragged onto a No plate cell", async ({ page }) => {
    await mockAnchorToday(page)
    const tag = uid()
    const slot = await seedSlot(`slot.noplatednd_${tag}`, "Moon", 996)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Laksa ${tag}`, role: "main" },
      tag
    )
    const day0 = dateOffset(0)
    const day3 = dateOffset(3)

    try {
      await seedPlateWithComponent(day0, slot.id, {
        food_id: food.id,
        portions: 1,
      })
      await setOverride(day3, slot.id, "none")

      await page.goto("/")
      const source = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      const target = page.locator(`[data-testid="cell-3-${slot.id}"]`)
      await expect(source.getByText(`Laksa ${tag}`)).toBeVisible()
      await expect(target.locator('[data-slot-state="none"]')).toBeVisible()

      const plateWrites: string[] = []
      page.on("request", (req) => {
        if (req.url().includes("/api/plates") && req.method() !== "GET") {
          plateWrites.push(`${req.method()} ${req.url()}`)
        }
      })

      // Same 6 px activation dance as planner-dnd.spec.ts.
      await source.scrollIntoViewIfNeeded()
      const from = await centerOf(source)
      const to = await centerOf(target)
      await page.mouse.move(from.x, from.y)
      await page.mouse.down()
      await page.mouse.move(from.x + 10, from.y, { steps: 5 })
      await page.mouse.move(to.x, to.y, { steps: 12 })
      await page.mouse.up()

      await expect(
        page.getByText("This meal is marked No plate.")
      ).toBeVisible()
      await expect(source.getByText(`Laksa ${tag}`)).toBeVisible()
      await expect(target.locator('[data-slot-state="none"]')).toBeVisible()
      expect(plateWrites).toEqual([])
    } finally {
      await clearOverride(day3, slot.id)
      await cleanupSlot(slot.id)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
    }
  })
})
