import {
  addDays,
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedFood,
  seedLeafFood,
  seedSlot,
  test,
  uid,
} from "./helpers"

// F5-AC3. Today asks about days already lived, one question at a time, so
// this test needs the real yesterday, where its plate (in a slot of its own
// that sorts after every other) is the latest one to rate. Copies of it
// would ask about each other's plates, so it runs in the serial
// `global-settings` project.

function localToday() {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function weekdayLong(iso: string) {
  const [y, m, d] = iso.split("-").map(Number)
  return new Intl.DateTimeFormat("en", { weekday: "long" }).format(
    new Date(y, m - 1, d)
  )
}

test("rating the last meal writes feedback; Undo clears it", async ({
  page,
}) => {
  const tag = uid()
  const yesterday = addDays(localToday(), -1)
  const slot = await seedSlot(`slot.tdy-rate-${tag}`, "Moon", 2_000_000)
  const beans = await seedLeafFood({ name: `Beans ${tag}`, kcal_100g: 100 })
  const name = `Chili ${tag}`
  const chili = await seedComposedFood({
    name,
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
    const plateRes = await ctx.post("/api/plates", {
      data: {
        date: yesterday,
        slot_id: slot.id,
        components: [{ food_id: chili.id }],
      },
    })
    expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
    await ctx.dispose()

    await page.goto("/today")
    const question = page.getByTestId("today-rate").filter({
      hasText: `How was ${weekdayLong(yesterday)}'s ${name}?`,
    })
    await expect(question).toBeVisible()

    const saved = page.waitForResponse(
      (r) =>
        /\/api\/plates\/\d+\/feedback$/.test(r.url()) &&
        r.request().method() === "PUT"
    )
    await question.getByRole("button", { name: "Loved it" }).click()
    const put = await saved
    expect(put.ok()).toBe(true)
    expect(put.request().postDataJSON()).toEqual({ status: "loved" })
    await expect(question).toHaveCount(0)

    const cleared = page.waitForResponse(
      (r) =>
        /\/api\/plates\/\d+\/feedback$/.test(r.url()) &&
        r.request().method() === "DELETE"
    )
    await page.getByRole("button", { name: "Undo" }).click()
    expect((await cleared).ok()).toBe(true)
    await expect(question).toBeVisible()
  } finally {
    await cleanupSlot(slot.id)
    await cleanupFood(chili.id)
    await cleanupFood(beans.id)
  }
})
