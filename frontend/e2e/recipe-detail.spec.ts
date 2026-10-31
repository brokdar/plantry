import {
  addDays,
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  openCellSheet,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// F9: recipe detail (portion stepper, scaled ingredients, history) and the
// editor's keep days and freezable.

function localToday() {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** Chili for 4 portions: 400 g beans. */
async function seedChili(tag: string, extra: Record<string, unknown> = {}) {
  const beans = await seedLeafFood({ name: `Beans ${tag}`, kcal_100g: 100 })
  const chili = await seedComposedFood({
    name: `Chili ${tag}`,
    role: "main",
    reference_portions: 4,
    children: [
      { child_id: beans.id, amount: 400, unit: "g", grams: 400, sort_order: 0 },
    ],
    ...extra,
  })
  return {
    beans,
    chili,
    cleanup: async () => {
      await cleanupFood(chili.id)
      await cleanupFood(beans.id)
    },
  }
}

test("F9-AC1: the stepper from 4 to 6 scales beans 400 g → 600 g with no write", async ({
  page,
}) => {
  const { chili, cleanup } = await seedChili(uid())
  const writes: string[] = []
  page.on("request", (r) => {
    if (r.url().includes("/api/") && r.method() !== "GET")
      writes.push(`${r.method()} ${r.url()}`)
  })
  try {
    await page.goto(`/recipes/${chili.id}`)
    const list = page.getByTestId("recipe-ingredients-list")
    await expect(list).toContainText("400 g")

    const more = page
      .getByTestId("recipe-portions")
      .getByRole("button", { name: "Increase Portions" })
    await more.click()
    await expect(list).toContainText("500 g")
    await more.click()
    await expect(list).toContainText("600 g")
    await expect(page.getByTestId("recipe-portions")).toContainText("6")
    expect(writes).toEqual([])

    await page.goto(`/recipes/${chili.id}?portions=2`)
    await expect(list).toContainText("200 g")
  } finally {
    await cleanup()
  }
})

test("F9-AC3: the history line matches B7's fields", async ({ page }) => {
  const tag = uid()
  const { chili, cleanup } = await seedChili(tag)
  const fresh = await seedChili(`${tag}-new`)
  // An inactive slot of its own: no board shows it and no other spec's
  // cleanup reaches its plates, but history counts them.
  const ctx = await apiRequest.newContext({ baseURL: API })
  const slotRes = await ctx.post("/api/settings/slots", {
    data: {
      name_key: `slot.detail-history-${tag}`,
      icon: "Moon",
      sort_order: 3_000_000,
      active: false,
    },
  })
  expect(slotRes.ok(), await slotRes.text()).toBeTruthy()
  const slot = (await slotRes.json()) as { id: number }
  try {
    const today = localToday()
    for (const date of [addDays(today, -40), addDays(today, -21)]) {
      const res = await ctx.post("/api/plates", {
        data: { date, slot_id: slot.id, components: [{ food_id: chili.id }] },
      })
      expect(res.ok(), await res.text()).toBeTruthy()
    }
    const food = (await (
      await ctx.get(`/api/foods/${chili.id}?today=${today}`)
    ).json()) as { times_cooked: number; last_eaten: string }
    expect(food).toMatchObject({
      times_cooked: 2,
      last_eaten: addDays(today, -21),
    })

    await page.goto(`/recipes/${chili.id}`)
    await expect(page.getByTestId("recipe-history")).toHaveText(
      "Cooked 2 times, last 3 weeks ago"
    )
    await page.goto(`/recipes/${fresh.chili.id}`)
    await expect(page.getByTestId("recipe-history")).toHaveText(
      "Not cooked yet"
    )
  } finally {
    await ctx.dispose()
    await cleanupSlot(slot.id)
    await cleanup()
    await fresh.cleanup()
  }
})

test("F9-AC2: keep days 5 warns, saves, and a new batch's use-by follows it", async ({
  page,
}) => {
  const tag = uid()
  const { chili, cleanup } = await seedChili(tag)
  const { start, days } = await futureWindow()
  const slot = await seedSlot(`slot.detail-keep-${tag}`, "Moon", 1_000_001)
  try {
    await page.goto(`/recipes/${chili.id}/edit`)
    const keep = page.getByLabel("Keeps (days)")
    await keep.fill("5")
    await expect(page.getByTestId("recipe-keep-days-warning")).toBeVisible()
    await page.getByLabel("Freezable").click()
    await page.getByRole("option", { name: "Yes" }).click()

    const saved = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/api/foods/${chili.id}`) &&
        r.request().method() === "PUT"
    )
    await page.getByRole("button", { name: /save/i }).click()
    expect((await saved).ok()).toBeTruthy()

    // The detail shows the new facts.
    const facts = page.getByTestId("recipe-facts")
    await expect(facts).toContainText("5 days")
    await expect(facts).toContainText("Freezable")

    // A new batch: cooked on day 1 with a leftover on day 2.
    const ctx = await apiRequest.newContext({ baseURL: API })
    const plateRes = await ctx.post("/api/plates", {
      data: {
        date: days[0],
        slot_id: slot.id,
        components: [{ food_id: chili.id }],
      },
    })
    expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
    const plate = (await plateRes.json()) as {
      components: { cook_session_id?: number }[]
    }
    const sessionId = plate.components[0].cook_session_id
    const leftovers = await ctx.post(
      `/api/cook-sessions/${sessionId}/leftovers`,
      {
        data: { cells: [{ date: days[1], slot_id: slot.id }] },
      }
    )
    expect(leftovers.ok(), await leftovers.text()).toBeTruthy()
    await ctx.dispose()

    await page.goto(`/plan?start=${start}`)
    const sheet = await openCellSheet(page, days[0], slot.id)
    const useBy = new Intl.DateTimeFormat("en", {
      weekday: "short",
      day: "numeric",
      month: "short",
    }).format(new Date(`${addDays(days[0], 5)}T00:00:00`))
    await expect(sheet.getByTestId("batch-section")).toContainText(
      `Use by ${useBy}`
    )
  } finally {
    await cleanupSlot(slot.id)
    await cleanup()
  }
})

test("F9-AC4: Cook now opens cook mode with the stepper's portions", async ({
  page,
}) => {
  const { chili, cleanup } = await seedChili(uid())
  try {
    await page.goto(`/recipes/${chili.id}`)
    await expect(page.getByTestId("recipe-ingredients-list")).toContainText(
      "400 g"
    )
    const more = page
      .getByTestId("recipe-portions")
      .getByRole("button", { name: "Increase Portions" })
    await more.click()
    await more.click()
    await page.getByRole("link", { name: "Cook now" }).click()
    await expect(page).toHaveURL(
      new RegExp(`/recipes/${chili.id}/cook\\?portions=6$`)
    )
    await expect(page.getByText("600 g")).toBeVisible()
  } finally {
    await cleanup()
  }
})
