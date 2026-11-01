import type { Locator, Page } from "@playwright/test"

import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  openCellSheet,
  planCell,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// Phase 3 F2: the review board without an assistant (D33, D34). Each test
// seeds its own slot in its own future window and makes drafts the way the
// generator leaves them: plates through the API, then Unlock (drafts are
// what Unlock returns a closure to). The Stew batch cooks on day 1 and is
// eaten again on day 2; the Bowl on day 3 is a batch of its own.

interface Setup {
  start: string
  days: string[]
  slot: number
  /** Plate ids: Stew cook (day 1), Stew leftover (day 2), Bowl (day 3). */
  cook: number
  leftover: number
  bowl: number
  cleanup: () => Promise<void>
}

async function setup(): Promise<Setup> {
  const tag = uid()
  const { start, days } = await futureWindow()
  const slot = await seedSlot(`slot.prh-${tag}`, "Moon", 99980)
  const base = await seedLeafFood({
    name: `Lentils ${tag}`,
    kcal_100g: 110,
    protein_100g: 9,
    fat_100g: 1,
    carbs_100g: 18,
  })
  const stew = await seedComposedFood({
    name: `Stew ${tag}`,
    role: "main",
    reference_portions: 2,
    keep_days: 3,
    children: [
      { child_id: base.id, amount: 400, unit: "g", grams: 400, sort_order: 0 },
    ],
  })
  const bowl = await seedComposedFood({
    name: `Bowl ${tag}`,
    role: "main",
    reference_portions: 1,
    children: [
      { child_id: base.id, amount: 200, unit: "g", grams: 200, sort_order: 0 },
    ],
  })

  const ctx = await apiRequest.newContext({ baseURL: API })
  async function plate(date: string, component: object): Promise<number> {
    const res = await ctx.post("/api/plates", {
      data: { date, slot_id: slot.id, eaters: 1, components: [component] },
    })
    const body = (await res.json()) as {
      id: number
      components: { cook_session_id?: number }[]
    }
    expect(res.ok(), JSON.stringify(body)).toBeTruthy()
    return body.id
  }
  const cook = await plate(days[0], { food_id: stew.id, portions: 1 })
  const cookRes = await ctx.get(`/api/plan?start=${start}`)
  const { sessions } = (await cookRes.json()) as {
    sessions: { id: number; food_id: number }[]
  }
  const session = sessions.find((s) => s.food_id === stew.id)
  expect(session, "the Stew batch").toBeTruthy()
  const leftover = await plate(days[1], {
    food_id: stew.id,
    portions: 1,
    cook_session_id: session!.id,
  })
  const bowlPlate = await plate(days[2], { food_id: bowl.id, portions: 1 })
  // Unlock turns each closure into drafts.
  for (const id of [cook, bowlPlate]) {
    const res = await ctx.post(`/api/plates/${id}/unlock`)
    expect(res.ok(), await res.text()).toBeTruthy()
  }
  await ctx.dispose()

  return {
    start,
    days,
    slot: slot.id,
    cook,
    leftover,
    bowl: bowlPlate,
    cleanup: async () => {
      await cleanupSlot(slot.id)
      await cleanupFood(stew.id)
      await cleanupFood(bowl.id)
      await cleanupFood(base.id)
    },
  }
}

const padlock = (cell: Locator) => cell.getByTestId("cell-padlock")

async function expectStates(
  page: Page,
  s: Setup,
  confirmed: [boolean, boolean, boolean]
) {
  for (const [i, locked] of confirmed.entries()) {
    const cell = planCell(page, s.days[i], s.slot)
    await expect(padlock(cell)).toHaveCount(locked ? 1 : 0)
  }
}

function waitForPost(page: Page, path: string) {
  return page.waitForResponse(
    (r) => r.url().includes(path) && r.request().method() === "POST"
  )
}

test.describe("plan review without the assistant", () => {
  test("F2-AC2/3: Confirm padlocks the batch with Undo; Unlock asks first", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      await expect(page.getByTestId("plan-summary")).toHaveText(
        "Draft · 3 plates · 2 cook days"
      )
      await expectStates(page, s, [false, false, false])

      // Confirm on the leftover confirms the whole Stew batch.
      let sheet = await openCellSheet(page, s.days[1], s.slot)
      const confirmed = waitForPost(page, `/api/plates/${s.leftover}/confirm`)
      await sheet.getByRole("button", { name: "Confirm" }).click()
      expect((await confirmed).ok()).toBeTruthy()
      await expect(sheet).toHaveCount(0)
      await expectStates(page, s, [true, true, false])

      // Undo unlocks it again.
      const undone = waitForPost(page, `/api/plates/${s.leftover}/unlock`)
      await page.getByRole("button", { name: "Undo" }).click()
      expect((await undone).ok()).toBeTruthy()
      await expectStates(page, s, [false, false, false])

      // Unlock asks first and names the rest of the batch; Cancel keeps it.
      sheet = await openCellSheet(page, s.days[1], s.slot)
      await sheet.getByRole("button", { name: "Confirm" }).click()
      await expectStates(page, s, [true, true, false])
      sheet = await openCellSheet(page, s.days[1], s.slot)
      await sheet.getByRole("button", { name: "Unlock" }).click()
      const dialog = page.getByRole("dialog", { name: "Unlock this plate?" })
      await expect(dialog).toContainText("Unlocking also unlocks:")
      await dialog.getByRole("button", { name: "Cancel" }).click()
      await expect(dialog).toHaveCount(0)
      await expectStates(page, s, [true, true, false])

      await sheet.getByRole("button", { name: "Unlock" }).click()
      const unlocked = waitForPost(page, `/api/plates/${s.leftover}/unlock`)
      await dialog.getByRole("button", { name: "Unlock" }).click()
      expect((await unlocked).ok()).toBeTruthy()
      await expectStates(page, s, [false, false, false])
    } finally {
      await s.cleanup()
    }
  })

  test("Accept and shop confirms the drafts and lands on Shop", async ({
    page,
  }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      const accepted = waitForPost(page, "/api/plan/accept")
      await page.getByRole("button", { name: "Accept and shop" }).click()
      expect((await accepted).ok()).toBeTruthy()
      await expect(page).toHaveURL(`/shop?from=${s.start}&to=${s.days[6]}`)
      await expect(
        page.getByText("Plan accepted. 3 plates confirmed.")
      ).toBeVisible()

      await page.goto(`/plan?start=${s.start}`)
      await expectStates(page, s, [true, true, true])
      await expect(page.getByTestId("plan-summary")).toHaveText(
        "3 plates, 2 cook days"
      )
      await expect(
        page.getByRole("button", { name: "Accept and shop" })
      ).toHaveCount(0)
    } finally {
      await s.cleanup()
    }
  })

  test("Undo of accept returns the plates to draft", async ({ page }) => {
    const s = await setup()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      await page.getByRole("button", { name: "Accept and shop" }).click()
      await expect(page).toHaveURL(/\/shop\?/)
      const unaccepted = waitForPost(page, "/api/plan/unaccept")
      await page.getByRole("button", { name: "Undo" }).click()
      expect((await unaccepted).ok()).toBeTruthy()

      await page.goto(`/plan?start=${s.start}`)
      await expectStates(page, s, [false, false, false])
      await expect(page.getByTestId("plan-summary")).toHaveText(
        "Draft · 3 plates · 2 cook days"
      )
    } finally {
      await s.cleanup()
    }
  })

  test("F2-AC6: Discard draft keeps confirmed plates, and Undo restores the drafts", async ({
    page,
  }) => {
    const s = await setup()
    try {
      // The Bowl is confirmed: discard leaves it.
      const ctx = await apiRequest.newContext({ baseURL: API })
      expect((await ctx.post(`/api/plates/${s.bowl}/confirm`)).ok()).toBe(true)
      await ctx.dispose()

      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/plan?start=${s.start}`)
      await page.getByRole("button", { name: "More plan actions" }).click()
      await page.getByRole("menuitem", { name: "Discard draft" }).click()
      const dialog = page.getByRole("dialog", { name: "Discard this draft?" })
      await expect(dialog).toContainText("This deletes 2 draft plates.")
      const discarded = waitForPost(page, "/api/plan/discard")
      await dialog.getByRole("button", { name: "Discard draft" }).click()
      expect((await discarded).ok()).toBeTruthy()

      const cells = [0, 1, 2].map((i) => planCell(page, s.days[i], s.slot))
      await expect(cells[0]).toHaveAttribute("data-cell-empty", "")
      await expect(cells[1]).toHaveAttribute("data-cell-empty", "")
      await expect(padlock(cells[2])).toHaveCount(1)
      await expect(page.getByTestId("plan-summary")).toHaveText(
        "1 plate, 1 cook day"
      )

      const restored = waitForPost(page, "/api/plan/restore")
      await page.getByRole("button", { name: "Undo" }).click()
      expect((await restored).ok()).toBeTruthy()
      await expectStates(page, s, [false, false, true])
      await expect(page.getByTestId("plan-summary")).toHaveText(
        "Draft · 2 plates · 1 cook day"
      )
    } finally {
      await s.cleanup()
    }
  })
})
