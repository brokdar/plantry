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

// F5-AC4 / AC-3 (Today part): a plate that eats frozen portions of a batch
// makes the day before it say "Take Chili out of the freezer tonight".
// `plan.freezing_enabled` is global, so this spec runs in the serial
// `global-settings` project and resets the setting however it ends.

test("the day before a frozen meal reminds to take it out of the freezer", async ({
  page,
}) => {
  const tag = uid()
  const ctx = await apiRequest.newContext({ baseURL: API })
  const seeded: { slot?: number; foods: number[] } = { foods: [] }
  try {
    const enabled = await ctx.put("/api/settings/plan.freezing_enabled", {
      data: { value: "true" },
    })
    expect(enabled.ok(), await enabled.text()).toBeTruthy()

    const { days } = await futureWindow()
    const slot = await seedSlot(`slot.tdyfz-${tag}`, "Moon", 1_000_000)
    seeded.slot = slot.id
    const beans = await seedLeafFood({ name: `Beans ${tag}`, kcal_100g: 100 })
    seeded.foods.push(beans.id)
    const name = `Chili ${tag}`
    const chili = await seedComposedFood({
      name,
      role: "main",
      reference_portions: 4,
      // Cooked on day 0, use by day 1: day 5 eats frozen portions.
      keep_days: 1,
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
    // Recipes before their ingredients: a used ingredient can't be deleted.
    seeded.foods.unshift(chili.id)

    const cookRes = await ctx.post("/api/plates", {
      data: {
        date: days[0],
        slot_id: slot.id,
        components: [{ food_id: chili.id }],
      },
    })
    expect(cookRes.ok(), await cookRes.text()).toBeTruthy()
    const cook = (await cookRes.json()) as {
      components: { cook_session_id: number }[]
    }
    const sessionId = cook.components[0].cook_session_id

    const frozenRes = await ctx.post("/api/plates", {
      data: {
        date: days[5],
        slot_id: slot.id,
        components: [{ food_id: chili.id, cook_session_id: sessionId }],
      },
    })
    expect(frozenRes.ok(), await frozenRes.text()).toBeTruthy()

    // Freeze what the frozen meal eats, as the cook strip would ask.
    const session = (await (
      await ctx.get(`/api/cook-sessions/${sessionId}`)
    ).json()) as {
      portions: number
      links: { kind: string; portions: number }[]
    }
    const frozen = session.links.find((l) => l.kind === "frozen")
    expect(frozen, JSON.stringify(session.links)).toBeTruthy()
    const put = await ctx.put(`/api/cook-sessions/${sessionId}`, {
      data: {
        portions: session.portions,
        frozen_portions: frozen!.portions,
        note: null,
      },
    })
    expect(put.ok(), await put.text()).toBeTruthy()

    await page.goto(`/today?date=${days[4]}`)
    const reminder = page.getByTestId("today-reminder-defrost")
    await expect(reminder).toContainText("Freezer")
    await expect(reminder).toContainText(
      `Take ${name} out of the freezer tonight`
    )

    // Only the day before: the frozen day itself has no defrost note.
    await page.goto(`/today?date=${days[5]}`)
    await expect(page.getByTestId("today-verdict")).toBeVisible()
    await expect(page.getByTestId("today-reminder-defrost")).toHaveCount(0)
  } finally {
    // Each step on its own, so one failure can't skip the setting reset.
    const steps: (() => Promise<unknown>)[] = [
      ...(seeded.slot !== undefined ? [() => cleanupSlot(seeded.slot!)] : []),
      ...seeded.foods.map((id) => () => cleanupFood(id)),
      () => ctx.delete("/api/settings/plan.freezing_enabled"),
    ]
    for (const step of steps) {
      await step().catch((err: unknown) =>
        console.warn("today-freezer cleanup:", err)
      )
    }
    await ctx.dispose()
  }
})
