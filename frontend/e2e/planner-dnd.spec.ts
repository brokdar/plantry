/**
 * Planner — drag-and-drop interaction
 *
 * Locks down the click-vs-drag contract on planned cells:
 *   - plain pointer-drag moves a plate to the target slot
 *   - Ctrl/Meta + drag copies the plate (source preserved, target gets a
 *     new plate id)
 *   - a short click on a planned cell opens the slot sheet — i.e. the
 *     `slot-open-sheet` button still wins when the pointer doesn't move
 *
 * dnd-kit's PointerSensor uses a 6 px `activationConstraint`, so each drag
 * issues a small intermediate move before traveling to the target. Without
 * that, the sensor never activates and Playwright's mouse just fires a
 * click instead.
 */

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
import type { Locator, Page } from "@playwright/test"

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

function dateOffset(offset: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offset)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

async function centerOf(loc: Locator): Promise<{ x: number; y: number }> {
  const box = await loc.boundingBox()
  if (!box) throw new Error("Locator has no bounding box")
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}

/**
 * Drag from `from` to `to`, satisfying dnd-kit's 6 px activation threshold.
 * The intermediate +10 px move is what wakes up the sensor; a single
 * `mouse.move` straight to the target is below threshold for the first
 * step and the drag never starts.
 */
async function dragViaMouse(
  page: Page,
  from: { x: number; y: number },
  to: { x: number; y: number }
) {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(from.x + 10, from.y, { steps: 5 })
  await page.mouse.move(to.x, to.y, { steps: 12 })
  await page.mouse.up()
}

/**
 * Ctrl/Meta + drag via synthetic PointerEvents dispatched in the page.
 *
 * Why not `page.keyboard.down("Control")` + `page.mouse.*`? In headless
 * Chromium under parallel workers, the CDP keyboard-state propagation
 * races the mouse pipeline — the activator pointerdown frequently arrives
 * with `ctrlKey: false`, so dnd-kit reads it as a plain move. Synthesizing
 * the events directly removes that race: the modifier is set as a
 * property on each event, deterministically. dnd-kit treats the events
 * identically (the activator is just a React listener on the wrapper).
 *
 * Coordinates are re-derived from selectors *inside* the page so a layout
 * shift between Playwright's boundingBox snapshot and event dispatch
 * (parallel tests adding rows to the shared grid) doesn't leave us
 * pointing at empty space.
 */
async function dragWithModifierViaEvents(
  page: Page,
  fromTestId: string,
  toTestId: string
) {
  await page.evaluate(
    async ({ fromTestId, toTestId }) => {
      function center(el: Element) {
        const r = el.getBoundingClientRect()
        return { x: r.left + r.width / 2, y: r.top + r.height / 2 }
      }
      const srcEl = document.querySelector(`[data-testid="${fromTestId}"]`)
      const tgtEl = document.querySelector(`[data-testid="${toTestId}"]`)
      if (!srcEl) throw new Error(`source ${fromTestId} not found`)
      if (!tgtEl) throw new Error(`target ${toTestId} not found`)
      const from = center(srcEl)
      const to = center(tgtEl)

      const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
      const mods = { ctrlKey: true, metaKey: true }
      function pe(type: string, x: number, y: number): PointerEvent {
        return new PointerEvent(type, {
          bubbles: true,
          cancelable: true,
          pointerType: "mouse",
          pointerId: 1,
          isPrimary: true,
          button: 0,
          buttons: type === "pointerup" ? 0 : 1,
          clientX: x,
          clientY: y,
          ...mods,
        })
      }
      function me(type: string, x: number, y: number): MouseEvent {
        return new MouseEvent(type, {
          bubbles: true,
          cancelable: true,
          button: 0,
          buttons: type === "mouseup" ? 0 : 1,
          clientX: x,
          clientY: y,
          ...mods,
        })
      }

      srcEl.dispatchEvent(pe("pointerdown", from.x, from.y))
      srcEl.dispatchEvent(me("mousedown", from.x, from.y))
      await sleep(20)
      document.dispatchEvent(pe("pointermove", from.x + 10, from.y))
      document.dispatchEvent(me("mousemove", from.x + 10, from.y))
      await sleep(20)

      const steps = 8
      for (let i = 1; i <= steps; i++) {
        const x = from.x + ((to.x - from.x) * i) / steps
        const y = from.y + ((to.y - from.y) * i) / steps
        document.dispatchEvent(pe("pointermove", x, y))
        document.dispatchEvent(me("mousemove", x, y))
        await sleep(10)
      }

      tgtEl.dispatchEvent(pe("pointerup", to.x, to.y))
      tgtEl.dispatchEvent(me("mouseup", to.x, to.y))
    },
    { fromTestId, toTestId }
  )
}

test.describe("Planner — drag-and-drop", () => {
  test.beforeEach(async ({ page }) => {
    await mockAnchorToday(page)
  })

  test("plain drag moves a plate to the target slot", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dndmove_${tag}`, "Moon", 940)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Pho ${tag}`, role: "main" },
      tag
    )
    const day0 = dateOffset(0)
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(day0, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")
      const source = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      const target = page.locator(`[data-testid="cell-3-${slot.id}"]`).first()
      await expect(source.getByText(`Pho ${tag}`)).toBeVisible()

      const moveResp = page.waitForResponse(
        (r) =>
          /\/api\/plates\/\d+$/.test(r.url()) && r.request().method() === "PUT"
      )
      await dragViaMouse(page, await centerOf(source), await centerOf(target))
      await moveResp

      await expect(target.getByText(`Pho ${tag}`)).toBeVisible()
      await expect(source.getByText(`Pho ${tag}`)).toHaveCount(0)
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("ctrl+drag copies the plate, leaving the source intact", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dndcopy_${tag}`, "Sun", 941)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Ramen ${tag}`, role: "main" },
      tag
    )
    const day0 = dateOffset(0)
    const cleanupIds: number[] = []

    try {
      const plate = await seedPlateByDate(day0, slot.id, food.id)
      cleanupIds.push(plate.id)

      await page.goto("/")
      const source = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      const target = page.locator(`[data-testid="cell-2-${slot.id}"]`).first()
      await expect(source.getByText(`Ramen ${tag}`)).toBeVisible()

      // Copy mode: PlannerGrid POSTs a fresh plate, then POSTs each component.
      // Wait for the plate POST so we know the new id and can clean it up.
      const createResp = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/plates") && r.request().method() === "POST"
      )
      await dragWithModifierViaEvents(
        page,
        `cell-0-${slot.id}`,
        `cell-2-${slot.id}`
      )
      const created = (await (await createResp).json()) as { id: number }
      cleanupIds.push(created.id)

      // Source remains populated — ctrl+drag is copy, not move.
      await expect(source.getByText(`Ramen ${tag}`)).toBeVisible()
      await expect(target.getByText(`Ramen ${tag}`)).toBeVisible()
      // The new plate at the target must have a different id than the source.
      expect(created.id).not.toBe(plate.id)
    } finally {
      for (const id of cleanupIds) await deletePlate(id)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })

  test("clicking a planned cell opens the slot sheet (no drag)", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dndclick_${tag}`, "Star", 942)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Curry ${tag}`, role: "main" },
      tag
    )
    const day0 = dateOffset(0)
    let plateId: number | undefined

    try {
      const plate = await seedPlateByDate(day0, slot.id, food.id)
      plateId = plate.id

      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell.getByText(`Curry ${tag}`)).toBeVisible()

      await cell.locator('[data-testid="slot-open-sheet"]').click()
      await expect(page.getByRole("dialog")).toBeVisible()
    } finally {
      if (plateId) await deletePlate(plateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
