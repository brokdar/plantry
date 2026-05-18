import { expect, test } from "./helpers"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  mockAnchorToday,
  seedLeafFood,
  seedSlot,
  uid,
} from "./helpers"

async function seedPlate(
  slotId: number,
  foodId: number,
  dayOffset: number = 0
): Promise<{ id: number }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const d = new Date()
  d.setDate(d.getDate() + dayOffset)
  const date = d.toISOString().slice(0, 10)
  const r = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId },
  })
  const plate = (await r.json()) as { id: number }
  await ctx.post(`/api/plates/${plate.id}/components`, {
    data: { food_id: foodId, amount: 100, unit: "g" },
  })
  await ctx.dispose()
  return plate
}

/**
 * Dispatches a synthetic touch-flavoured PointerEvent stream to drive the
 * MobileSlotRow gesture state machine. Playwright's `page.touchscreen` only
 * exposes `tap`, so swipe and long-press flows are scripted directly.
 *
 * Captures the element under the finger at pointerdown and dispatches every
 * subsequent move/up on that same element. Mirrors real touch behaviour:
 * browsers implicitly capture touch on the first target, so the row's
 * handlers keep receiving events with updated `clientX`/`clientY` even after
 * the finger crosses into a different DOM node (e.g. a day tab).
 */
type Step = {
  type: "down" | "move" | "up"
  x: number
  y: number
  wait?: number
}
async function pointerScript(
  page: import("@playwright/test").Page,
  steps: Step[]
) {
  await page.evaluate(async (steps) => {
    interface Globals {
      __pointerTarget?: Element
    }
    const g = window as unknown as Globals
    const first = steps[0]
    if (!first) throw new Error("script must include at least one step")
    if (first.type === "down") {
      const target = document.elementFromPoint(first.x, first.y)
      if (!target) throw new Error(`no element at (${first.x}, ${first.y})`)
      g.__pointerTarget = target
    }
    const target = g.__pointerTarget
    if (!target)
      throw new Error("no captured pointer target — start with 'down'")
    function fire(type: string, x: number, y: number) {
      target!.dispatchEvent(
        new PointerEvent(type, {
          pointerId: 1,
          pointerType: "touch",
          clientX: x,
          clientY: y,
          bubbles: true,
          cancelable: true,
        })
      )
    }
    for (const s of steps) {
      const evType =
        s.type === "down"
          ? "pointerdown"
          : s.type === "up"
            ? "pointerup"
            : "pointermove"
      fire(evType, s.x, s.y)
      if (s.wait) await new Promise((r) => setTimeout(r, s.wait))
    }
  }, steps)
}

test.describe("Mobile planner (day-tab layout)", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true })

  test("renders day tabs and switches the visible day", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.breakfast_${tag}`, "Coffee", 994)

    try {
      await page.goto("/")

      // All 7 day tabs visible on mobile; desktop grid is hidden at this size.
      for (let i = 0; i < 7; i++) {
        await expect(page.getByTestId(`mobile-day-tab-${i}`)).toBeVisible()
      }

      // Switching the active day updates aria-selected.
      const wed = page.getByTestId("mobile-day-tab-2")
      await wed.click()
      await expect(wed).toHaveAttribute("aria-selected", "true")

      // Every active-day slot is reachable — seeded slot appears with its empty
      // placeholder button.
      await expect(page.getByTestId(`mobile-cell-2-${slot.id}`)).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("swipe-left reveals action drawer; tap Skip marks slot skipped", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Soup", 995)
    const food = await seedLeafFood({ name: `Stew ${tag}` })
    const plate = await seedPlate(slot.id, food.id, 0)

    try {
      await mockAnchorToday(page)
      await page.goto("/")

      const cell = page.getByTestId(`mobile-cell-0-${slot.id}`)
      await expect(cell).toBeVisible()
      await expect(cell.getByText(`Stew ${tag}`)).toBeVisible()

      const row = page.getByTestId(`mobile-slot-row-${slot.id}`)
      // Mobile day list is scroll-virtualized below the day-tab strip — the
      // seeded slot row may sit far below the fold. Scroll it on-screen so
      // boundingBox() and document.elementFromPoint(...) agree on coords.
      await row.scrollIntoViewIfNeeded()
      const box = await row.boundingBox()
      if (!box) throw new Error("row has no bounding box")
      const x = box.x + box.width / 2
      const y = box.y + box.height / 2

      // Quick lateral swipe — moves > 8 px horizontally before vertical drift,
      // so the row's gesture decider locks into "swipe" mode.
      await pointerScript(page, [
        { type: "down", x, y },
        { type: "move", x: x - 30, y },
        { type: "move", x: x - 80, y },
        { type: "move", x: x - 140, y },
        { type: "up", x: x - 140, y },
      ])

      // Drawer marker appears on the inner card wrapper once snapped open.
      const cardWrapper = row.locator("[data-mobile-row-state]")
      await expect(cardWrapper).toHaveAttribute("data-drawer-open", "true")

      const skipResp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/plates/${plate.id}/skip`) &&
          r.request().method() === "POST"
      )
      await row.getByTestId("mobile-row-skip").click()
      await skipResp

      // The card now renders the skipped variant.
      await expect(cell.locator('[data-slot-state="skipped"]')).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
      await cleanupFood(food.id)
    }
  })

  test("long-press + drop on another day tab moves the plate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Soup", 996)
    const food = await seedLeafFood({ name: `Bisque ${tag}` })
    const plate = await seedPlate(slot.id, food.id, 0)

    try {
      await mockAnchorToday(page)
      await page.goto("/")

      // The seeded plate lives on tab 0 (mocked anchor: window starts today).
      await expect(
        page.getByTestId(`mobile-cell-0-${slot.id}`).getByText(`Bisque ${tag}`)
      ).toBeVisible()

      const row = page.getByTestId(`mobile-slot-row-${slot.id}`)
      // Bring the slot row into view so its boundingBox returns valid
      // viewport coordinates for the pointerdown step. The day-tab strip
      // is *not* sticky in the mobile layout, so depending on slot-list
      // length (parallel tests seed extra slots) it can be pushed off the
      // viewport top by the scroll. We rescue it before phase 2 below.
      await row.scrollIntoViewIfNeeded()
      const rBox = await row.boundingBox()
      if (!rBox) throw new Error("row has no bounding box")
      const x0 = rBox.x + rBox.width / 2
      const y0 = rBox.y + rBox.height / 2

      const target = page.getByTestId("mobile-day-tab-2")

      const updateResp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/plates/${plate.id}`) &&
          r.request().method() === "PUT"
      )

      // Phase 1: pointerdown alone, then wait for the long-press timer
      // (380 ms in the row) to flip the gesture into "drag". Splitting the
      // press from the moves into separate page.evaluate calls guarantees
      // React commits the drag-mode state before any subsequent move
      // events are dispatched on the same target.
      await pointerScript(page, [{ type: "down", x: x0, y: y0, wait: 500 }])

      const cardWrapper = row.locator("[data-mobile-row-state]")
      await expect(cardWrapper).toHaveAttribute("data-mobile-row-state", "drag")

      // Bring the day-tab strip on-screen now that drag mode is active —
      // synthetic pointermoves still fire on the captured target (the row,
      // possibly now off-screen), but hitTestDayTab uses
      // document.elementsFromPoint(clientX, clientY) which only finds the
      // tab if it currently sits at those viewport coords.
      await target.scrollIntoViewIfNeeded()
      const tBox = await target.boundingBox()
      if (!tBox) throw new Error("day tab has no bounding box")
      const xT = tBox.x + tBox.width / 2
      const yT = tBox.y + tBox.height / 2
      const hitsTab = await page.evaluate(
        ({ x, y }) =>
          document
            .elementsFromPoint(x, y)
            .some((el) => (el as HTMLElement).dataset?.mobileDayDrop === "2"),
        { x: xT, y: yT }
      )
      expect(
        hitsTab,
        `elementsFromPoint(${xT}, ${yT}) did not hit mobile-day-tab-2 — coords are stale or covered`
      ).toBe(true)

      // Phase 2: move toward the day tab. After the moves the hovered day
      // tab should be marked drop-hovered — assert that before releasing so
      // a hit-test mismatch surfaces here instead of as an opaque PUT
      // timeout downstream.
      await pointerScript(page, [
        { type: "move", x: x0 + (xT - x0) * 0.25, y: y0 + (yT - y0) * 0.25 },
        { type: "move", x: x0 + (xT - x0) * 0.5, y: y0 + (yT - y0) * 0.5 },
        { type: "move", x: x0 + (xT - x0) * 0.75, y: y0 + (yT - y0) * 0.75 },
        { type: "move", x: xT, y: yT },
      ])
      await expect(target).toHaveAttribute("data-drop-hovered", "true")

      // Phase 3: release on top of the day tab — fires the PUT.
      await pointerScript(page, [{ type: "up", x: xT, y: yT }])

      await updateResp

      // Active day follows the plate; Wednesday cell now carries the food.
      await expect(target).toHaveAttribute("aria-selected", "true")
      await expect(
        page.getByTestId(`mobile-cell-2-${slot.id}`).getByText(`Bisque ${tag}`)
      ).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
      await cleanupFood(food.id)
    }
  })

  test("planned row shows grip glyph and short drawer labels", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Soup", 997)
    const food = await seedLeafFood({ name: `Risotto ${tag}` })
    await seedPlate(slot.id, food.id, 0)

    try {
      await mockAnchorToday(page)
      await page.goto("/")

      const row = page.getByTestId(`mobile-slot-row-${slot.id}`)
      // Grip glyph signals the swipe + long-press affordance to sighted users.
      await expect(row.getByTestId("mobile-row-grip")).toBeVisible()
      // Sr-only hint mirrors that affordance for assistive tech.
      await expect(row.getByTestId("mobile-row-gesture-hint")).toContainText(
        /swipe|wische/i
      )
      // Outer wrapper is labelled as a row-actions surface.
      await expect(row).toHaveAttribute("aria-label", /row actions|zeilen/i)

      // Drawer button labels are the short verb form; aria-label keeps the
      // descriptive long form for screen readers.
      const skip = row.getByTestId("mobile-row-skip")
      await expect(skip).toHaveAttribute("aria-label", /skip|auslassen/i)
      await expect(skip).toContainText(/^skip$|^auslassen$/i)
    } finally {
      await cleanupSlot(slot.id)
      await cleanupFood(food.id)
    }
  })

  test("Move-to-day picker in slot sheet relocates the plate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.lunch_${tag}`, "Soup", 998)
    const food = await seedLeafFood({ name: `Curry ${tag}` })
    const plate = await seedPlate(slot.id, food.id, 0)

    try {
      await mockAnchorToday(page)
      await page.goto("/")

      const cell = page.getByTestId(`mobile-cell-0-${slot.id}`)
      await expect(cell.getByText(`Curry ${tag}`)).toBeVisible()

      // Open the slot sheet by tapping the planned card.
      await cell.getByTestId("slot-open-sheet").click()
      const picker = page.getByTestId("slot-sheet-move-picker")
      await expect(picker).toBeVisible()

      // Today (weekday 2 if today is Wed; not a stable assumption here). Use
      // the day chip whose aria-current="true" as the source, and any other
      // chip as the destination.
      const currentChip = picker.locator('[aria-current="true"]')
      const currentLabel = (await currentChip.getAttribute("aria-label")) ?? ""
      // Pick the first chip that is not the current one.
      const destChip = picker
        .locator("button")
        .filter({ hasNotText: currentLabel.split(" ")[0]! })
        .first()

      const updateResp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/plates/${plate.id}`) &&
          r.request().method() === "PUT"
      )
      await destChip.click()
      await updateResp

      // Sheet closes after a successful move.
      await expect(page.getByTestId("slot-sheet")).toHaveCount(0)
    } finally {
      await cleanupSlot(slot.id)
      await cleanupFood(food.id)
    }
  })
})
