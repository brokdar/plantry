import type { Page } from "@playwright/test"

import {
  API,
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

// F6: cook mode (UX §5.3) on the AC-1 recipe: Chili for 4 with 400 g kidney
// beans and 2 onions (pieces), cooked as a batch of 6 in this worker's own
// future window. The batch is sized with an explicit PUT (D11: explicit
// edits win), so its portions don't depend on the global household size.

const STEPS = [
  "Chop the onions.",
  "Fry the onions for 5 min.",
  "Add the beans.",
  "Simmer 20 Minuten.",
  "Season and serve.",
]

interface Seed {
  recipeId: number
  name: string
  sessionId: number
  cookDay: string
  cleanup: () => Promise<void>
}

async function seedBatch(): Promise<Seed> {
  const tag = uid()
  const { days } = await futureWindow()
  const cookDay = days[1]
  const slot = await seedSlot(`slot.cook-${tag}`, "Moon", 1_000_000)
  const beans = await seedLeafFood({ name: `Kidney beans ${tag}` })
  const onion = await seedLeafFood({ name: `Onion ${tag}` })
  const name = `Chili ${tag}`
  const chili = await seedComposedFood({
    name,
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
    instructions: STEPS.map((text, i) => ({ step_number: i + 1, text })),
  })

  const ctx = await apiRequest.newContext({ baseURL: API })
  const plateRes = await ctx.post("/api/plates", {
    data: {
      date: cookDay,
      slot_id: slot.id,
      components: [{ food_id: chili.id, portions: 1 }],
    },
  })
  expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
  const plate = (await plateRes.json()) as {
    components: { cook_session_id: number }[]
  }
  const sessionId = plate.components[0].cook_session_id
  const put = await ctx.put(`/api/cook-sessions/${sessionId}`, {
    data: { portions: 6, frozen_portions: 0, note: null },
  })
  expect(put.ok(), await put.text()).toBeTruthy()
  await ctx.dispose()

  return {
    recipeId: chili.id,
    name,
    sessionId,
    cookDay,
    cleanup: async () => {
      await cleanupSlot(slot.id)
      await cleanupFood(chili.id)
      await cleanupFood(beans.id)
      await cleanupFood(onion.id)
    },
  }
}

async function getSession(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(`/api/cook-sessions/${id}`)
  expect(res.ok()).toBeTruthy()
  const body = (await res.json()) as {
    portions: number
    cooked_at: string | null
  }
  await ctx.dispose()
  return body
}

/** The ingredient row for `name`. */
function ingredient(page: Page, name: string) {
  return page.getByTestId("cook-ingredient").filter({ hasText: name })
}

/** Records every write the page sends to the API. */
function recordWrites(page: Page): string[] {
  const writes: string[] = []
  page.on("request", (req) => {
    if (req.url().includes("/api/") && req.method() !== "GET") {
      writes.push(`${req.method()} ${req.url()}`)
    }
  })
  return writes
}

test.describe("Cook mode", () => {
  // F6-AC1
  test("scales the batch and persists a new portion count", async ({
    page,
  }) => {
    const seed = await seedBatch()
    try {
      await page.setViewportSize({ width: 375, height: 812 })
      await page.goto(`/cook/${seed.sessionId}`)

      await expect(
        page.getByRole("heading", { level: 1, name: seed.name })
      ).toBeVisible()
      await expect(page.getByTestId("cook-badge")).toHaveText("Cooking × 6")
      await expect(ingredient(page, "Kidney beans")).toContainText("600 g")
      await expect(ingredient(page, "Onion")).toContainText("3 pcs")

      const put = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/api/cook-sessions/${seed.sessionId}`) &&
          r.request().method() === "PUT"
      )
      const more = page.getByRole("button", { name: "Increase Portions" })
      await more.click()
      await more.click()
      await expect(page.getByTestId("cook-badge")).toHaveText("Cooking × 8")
      await expect(ingredient(page, "Kidney beans")).toContainText("800 g")
      await expect(ingredient(page, "Onion")).toContainText("4 pcs")
      const res = await put
      expect(res.ok()).toBeTruthy()
      expect(res.request().postDataJSON()).toMatchObject({ portions: 8 })

      await page.reload()
      await expect(page.getByTestId("cook-badge")).toHaveText("Cooking × 8")
      await expect(ingredient(page, "Kidney beans")).toContainText("800 g")
      expect((await getSession(seed.sessionId)).portions).toBe(8)
    } finally {
      await seed.cleanup()
    }
  })

  // F6-AC3
  test("the step pager moves with buttons and arrow keys", async ({ page }) => {
    const seed = await seedBatch()
    try {
      await page.setViewportSize({ width: 375, height: 812 })
      await page.goto(`/cook/${seed.sessionId}`)
      await page.getByRole("tab", { name: "Steps" }).click()

      const position = page.getByTestId("step-position")
      await expect(position).toHaveText("Step 1 of 5")
      await page.getByRole("button", { name: "Next" }).click()
      await expect(position).toHaveText("Step 2 of 5")
      await expect(page.getByTestId("step-text")).toContainText(
        "Fry the onions"
      )

      // The duration is a timer button; its pill stays across steps.
      const start = page.getByRole("button", { name: "Start timer: 5 min" })
      await start.click()
      await expect(page.getByTestId("cook-timer")).toContainText(
        "Step 2 · 5 min"
      )
      // A second press while it runs starts no duplicate.
      await start.click()
      await expect(page.getByTestId("cook-timer")).toHaveCount(1)

      await page.keyboard.press("ArrowRight")
      await expect(position).toHaveText("Step 3 of 5")
      await expect(page.getByTestId("cook-timer")).toBeVisible()
      await page.keyboard.press("ArrowLeft")
      await expect(position).toHaveText("Step 2 of 5")

      // Closing with a running timer asks first.
      await page.getByRole("link", { name: "Close cook mode" }).click()
      const leave = page.getByRole("dialog", { name: "Leave cook mode?" })
      await leave.getByRole("button", { name: "Keep cooking" }).click()
      await expect(leave).toHaveCount(0)
      await expect(page).toHaveURL(new RegExp(`/cook/${seed.sessionId}$`))
      await page.getByRole("link", { name: "Close cook mode" }).click()
      await leave.getByRole("button", { name: "Leave" }).click()
      await expect(page).toHaveURL(/\/today$/)
    } finally {
      await seed.cleanup()
    }
  })

  // F6-AC4
  test("Done cooking marks the batch cooked and Today says Cooked", async ({
    page,
  }) => {
    const seed = await seedBatch()
    try {
      await page.goto(`/cook/${seed.sessionId}`)
      for (let i = 1; i < STEPS.length; i++) {
        await page.getByRole("button", { name: "Next" }).click()
      }
      await expect(page.getByTestId("step-position")).toHaveText("Step 5 of 5")

      const cooked = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/api/cook-sessions/${seed.sessionId}/cooked`) &&
          r.request().method() === "POST"
      )
      await page.getByRole("button", { name: "Done cooking" }).click()
      expect((await cooked).ok()).toBeTruthy()
      await expect(page).toHaveURL(/\/today$/)
      expect((await getSession(seed.sessionId)).cooked_at).not.toBeNull()

      await page.goto(`/today?date=${seed.cookDay}`)
      const card = page.getByTestId("today-cook-card")
      await expect(card.getByTestId("today-cooked")).toHaveText("Cooked")
      await expect(card.getByTestId("today-start-cooking")).toHaveCount(0)
      // A cooked batch no longer heads the day.
      await expect(page.getByTestId("today-verdict")).not.toContainText(
        seed.name
      )
    } finally {
      await seed.cleanup()
    }
  })

  // F6-AC5
  test("cooking a recipe scales locally and writes nothing", async ({
    page,
  }) => {
    const seed = await seedBatch()
    try {
      const writes = recordWrites(page)
      await page.goto(`/recipes/${seed.recipeId}/cook?portions=2`)

      await expect(page.getByTestId("cook-badge")).toHaveText("Cooking × 2")
      await expect(ingredient(page, "Kidney beans")).toContainText("200 g")
      await expect(ingredient(page, "Onion")).toContainText("1 pcs")
      await expect(page.getByTestId("cook-coverage")).toHaveCount(0)

      await page.getByRole("button", { name: "Increase Portions" }).click()
      await expect(ingredient(page, "Kidney beans")).toContainText("300 g")

      for (let i = 1; i < STEPS.length; i++) {
        await page.getByRole("button", { name: "Next" }).click()
      }
      await page.getByRole("button", { name: "Done cooking" }).click()
      await expect(page).toHaveURL(new RegExp(`/recipes/${seed.recipeId}$`))
      expect(writes).toEqual([])
    } finally {
      await seed.cleanup()
    }
  })

  // F6-AC6
  test("renders without the wake lock API", async ({ page }) => {
    const seed = await seedBatch()
    try {
      const errors: string[] = []
      page.on("pageerror", (e) => errors.push(e.message))
      await page.addInitScript(() => {
        Object.defineProperty(Navigator.prototype, "wakeLock", {
          get: () => undefined,
          configurable: true,
        })
      })
      await page.goto(`/cook/${seed.sessionId}`)
      await expect(ingredient(page, "Kidney beans")).toContainText("600 g")
      expect(await page.evaluate(() => navigator.wakeLock)).toBeUndefined()
      expect(errors).toEqual([])
    } finally {
      await seed.cleanup()
    }
  })
})
