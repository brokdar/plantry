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

// F5: Today (UX §5.2) on the AC-1 batch, seeded through the API in this
// worker's own future window: Chili cooked on day 1 in a slot that sorts
// last, eaten again on day 2 at "lunch" and "dinner" (slots that sort
// first), so the batch has three meals: Cook × 3H with leftovers 2/3 and
// 3/3. Every plate gets the household's eaters H (D29), which is a global
// setting, so the portions come from the session the server returns.

interface Ac1 {
  /** Batch portions: 3 meals × household eaters. */
  portions: number
  /** Portions one leftover plate eats (its eaters). */
  perMeal: number
  name: string
  sessionId: number
  cookDay: string
  leftoverDay: string
  useBy: string
  lunch: number
  dinner: number
  cleanup: () => Promise<void>
}

async function seedAc1(): Promise<Ac1> {
  const tag = uid()
  const { days } = await futureWindow()
  const [cookDay, leftoverDay] = [days[1], days[2]]
  const lunch = await seedSlot(`slot.tdy-lunch-${tag}`, "Sun", -2)
  const dinner = await seedSlot(`slot.tdy-dinner-${tag}`, "Moon", -1)
  const cook = await seedSlot(`slot.tdy-cook-${tag}`, "Moon", 1_000_000)
  const beans = await seedLeafFood({
    name: `Beans ${tag}`,
    kcal_100g: 100,
    protein_100g: 8,
    fat_100g: 1,
    carbs_100g: 15,
  })
  const name = `Chili ${tag}`
  const chili = await seedComposedFood({
    name,
    role: "main",
    reference_portions: 4,
    keep_days: 3,
    children: [
      { child_id: beans.id, amount: 400, unit: "g", grams: 400, sort_order: 0 },
    ],
  })

  const ctx = await apiRequest.newContext({ baseURL: API })
  const plateRes = await ctx.post("/api/plates", {
    data: {
      date: cookDay,
      slot_id: cook.id,
      components: [{ food_id: chili.id, portions: 1 }],
    },
  })
  expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
  const plate = (await plateRes.json()) as {
    components: { cook_session_id: number }[]
  }
  const sessionId = plate.components[0].cook_session_id
  const leftovers = await ctx.post(
    `/api/cook-sessions/${sessionId}/leftovers`,
    {
      data: {
        cells: [
          { date: leftoverDay, slot_id: lunch.id },
          { date: leftoverDay, slot_id: dinner.id },
        ],
      },
    }
  )
  expect(leftovers.ok(), await leftovers.text()).toBeTruthy()
  const session = (await leftovers.json()) as {
    portions: number
    use_by: string
    meals: number
    links: { portions: number }[]
  }
  expect(session.meals).toBe(3)
  await ctx.dispose()

  return {
    name,
    sessionId,
    portions: session.portions,
    perMeal: session.links[1].portions,
    cookDay,
    leftoverDay,
    useBy: session.use_by,
    lunch: lunch.id,
    dinner: dinner.id,
    cleanup: async () => {
      for (const id of [cook.id, lunch.id, dinner.id]) await cleanupSlot(id)
      await cleanupFood(chili.id)
      await cleanupFood(beans.id)
    },
  }
}

function weekday(iso: string, style: "short" | "long") {
  const [y, m, d] = iso.split("-").map(Number)
  return new Intl.DateTimeFormat("en", { weekday: style }).format(
    new Date(y, m - 1, d)
  )
}
const weekdayShort = (iso: string) => weekday(iso, "short")
const weekdayLong = (iso: string) => weekday(iso, "long")

test.describe("Today", () => {
  // F5-AC1 (AC-1 steps 4–5).
  test("the leftover day and the cook day of the AC-1 batch", async ({
    page,
  }) => {
    const ac1 = await seedAc1()
    try {
      await page.setViewportSize({ width: 375, height: 812 })
      await page.goto(`/today?date=${ac1.leftoverDay}`)

      // Not today: the headline names the day.
      await expect(page.getByTestId("today-verdict")).toHaveText(
        `Nothing to cook on ${weekdayLong(ac1.leftoverDay)}`
      )
      const card = page.getByTestId("today-leftover-card")
      await expect(
        card.getByRole("heading", { name: `${ac1.name}, leftover` })
      ).toBeVisible()
      await expect(card.locator("[data-batch-badge]")).toHaveText(
        `2/3 · use by ${weekdayShort(ac1.useBy)}`
      )
      await expect(card).toContainText(
        ac1.perMeal === 1
          ? "Reheat 1 portion"
          : `Reheat ${ac1.perMeal} portions`
      )
      const dinner = page.getByTestId(`today-meal-${ac1.dinner}`)
      await expect(dinner).toContainText(ac1.name)
      await expect(dinner.locator("[data-batch-badge]")).toHaveText("3/3")

      // The previous-day button walks back to the cook day.
      await page.getByRole("link", { name: "Previous day" }).click()
      await expect(page).toHaveURL(new RegExp(`date=${ac1.cookDay}$`))
      await expect(page.getByTestId("today-verdict")).toHaveText(
        `Cook ${ac1.name} × ${ac1.portions} on ${weekdayLong(ac1.cookDay)}`
      )
      await expect(
        page
          .getByTestId("today-cook-card")
          .getByRole("link", { name: "Start cooking" })
      ).toHaveAttribute("href", `/cook/${ac1.sessionId}`)
    } finally {
      await ac1.cleanup()
    }
  })

  // Phase 3 F5-AC2 (AC-6 bullet 1): a window holding only drafts.
  test("a draft-only week says so and Review plan opens it", async ({
    page,
  }) => {
    const tag = uid()
    const { start, days } = await futureWindow()
    const slot = await seedSlot(`slot.tdy-draft-${tag}`, "Moon", 1_000_000)
    const beans = await seedLeafFood({ name: `Beans ${tag}` })
    const chili = await seedComposedFood({
      name: `Chili ${tag}`,
      role: "main",
      reference_portions: 4,
      children: [
        {
          child_id: beans.id,
          amount: 400,
          unit: "g",
          grams: 400,
          sort_order: 0,
        },
      ],
    })
    try {
      const ctx = await apiRequest.newContext({ baseURL: API })
      const plate = await ctx.post("/api/plates", {
        data: {
          date: days[2],
          slot_id: slot.id,
          components: [{ food_id: chili.id, portions: 1 }],
        },
      })
      expect(plate.ok(), await plate.text()).toBeTruthy()
      const { id } = (await plate.json()) as { id: number }
      // B1: unlocking turns the plate and its batch back into a draft.
      const unlock = await ctx.post(`/api/plates/${id}/unlock`)
      expect(unlock.ok(), await unlock.text()).toBeTruthy()
      await ctx.dispose()

      // That window's day is today, so the headline speaks of this week.
      await page.clock.setFixedTime(new Date(`${days[2]}T12:00:00`))
      await page.goto("/today")
      await expect(page.getByTestId("today-verdict")).toHaveText(
        "Your plan for this week is still a draft"
      )
      const card = page.getByTestId("today-draft-only")
      // The cook card is confirmed only (D26); the row shows the draft,
      // marked, as the week strip does (DoD should-fix 1).
      await expect(page.getByTestId("today-cook-card")).toHaveCount(0)
      const row = page.getByTestId(`today-meal-${slot.id}`)
      await expect(row).toContainText(chili.name)
      await expect(row.getByTestId("draft-tag")).toHaveText("Draft")
      await expect(row).not.toContainText("Nothing planned")
      await card.getByRole("link", { name: "Review plan" }).click()
      await expect(page).toHaveURL(`/plan?start=${start}`)
    } finally {
      await cleanupSlot(slot.id)
      await cleanupFood(chili.id)
      await cleanupFood(beans.id)
    }
  })

  test("desktop: the week strip opens the plan at that day", async ({
    page,
  }) => {
    const ac1 = await seedAc1()
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(`/today?date=${ac1.leftoverDay}`)
      const strip = page.getByRole("navigation", { name: "This week" })
      const cookDay = strip.getByTestId(`week-strip-${ac1.cookDay}`)
      await expect(cookDay.locator("[data-batch-badge]").first()).toHaveText(
        `Cook × ${ac1.portions}`
      )
      await cookDay.click()
      await expect(page).toHaveURL(new RegExp(`day=${ac1.cookDay}`))
      await expect(page.getByRole("grid")).toBeVisible()
    } finally {
      await ac1.cleanup()
    }
  })
})
