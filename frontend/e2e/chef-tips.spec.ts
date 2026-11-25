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
  setSetting,
  test,
  uid,
} from "./helpers"

/**
 * A5 tips on a phone (TIP-4, AC-5): a rich plate with nothing sharp gets a
 * finishing-acid tip; Apply adds the lemon to the plate (cook mode lists it
 * under "To finish", the shopping list gains it) and Undo takes it back.
 * chef.enabled is process-global, so this spec runs in the global-settings
 * project and clears the override it writes.
 */

test.use({ viewport: { width: 390, height: 844 } })

async function clearChefEnabled() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete("/api/settings/chef.enabled")
  await ctx.dispose()
}

test.afterEach(clearChefEnabled)

test("apply a garnish tip, see it in cook mode, undo", async ({ page }) => {
  const tag = uid()
  const { start, days } = await futureWindow()
  const day = days[1]
  const slot = await seedSlot(`slot.tip-${tag}`, "Moon", 1_100_000)
  const pasta = await seedLeafFood({
    name: `Spaghetti ${tag}`,
    kcal_100g: 350,
    protein_100g: 12,
    fat_100g: 2,
    carbs_100g: 70,
  })
  const cream = await seedLeafFood({
    name: `Sahne ${tag}`,
    kcal_100g: 300,
    protein_100g: 2,
    fat_100g: 30,
    carbs_100g: 3,
  })
  const lemon = await seedLeafFood({
    name: `Zitrone ${tag}`,
    kcal_100g: 30,
    protein_100g: 1,
    fat_100g: 0.3,
    carbs_100g: 3,
    fiber_100g: 2,
  })
  const main = await seedComposedFood({
    name: `Pasta panna ${tag}`,
    role: "main",
    reference_portions: 1,
    children: [
      { child_id: pasta.id, amount: 120, unit: "g", grams: 120, sort_order: 0 },
      { child_id: cream.id, amount: 80, unit: "g", grams: 80, sort_order: 1 },
    ],
    instructions: [{ step_number: 1, text: "Cook the pasta." }],
  })
  const ctx = await apiRequest.newContext({ baseURL: API })
  try {
    // Attributes as the user set them: a rich, creamy Italian main with no
    // acid, and a sharp lemon garnish.
    for (const [id, attrs] of [
      [
        main.id,
        {
          cuisine: "italian",
          texture: "creamy",
          temperature: "hot",
          colour: "beige",
          richness: 3,
          saucy: false,
          taste: { salt: 2, acid: 0, sweet: 0, bitter: 0, umami: 2, heat: 0 },
        },
      ],
      [
        lemon.id,
        {
          texture: "soft",
          colour: "beige",
          taste: { salt: 0, acid: 3, sweet: 0, bitter: 0, umami: 0, heat: 0 },
        },
      ],
    ] as const) {
      const res = await ctx.put(`/api/foods/${id}/attributes`, { data: attrs })
      expect(res.ok(), await res.text()).toBeTruthy()
    }
    const plateRes = await ctx.post("/api/plates", {
      data: {
        date: day,
        slot_id: slot.id,
        components: [{ food_id: main.id, portions: 1 }],
      },
    })
    expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
    const plate = (await plateRes.json()) as {
      id: number
      components: { cook_session_id: number }[]
    }
    const sessionId = plate.components[0].cook_session_id
    await setSetting("chef.enabled", "true")

    const shop = async () => {
      const res = await ctx.get(
        `/api/shopping-list?from=${day}&to=${day}&drafts=1`
      )
      return JSON.stringify(await res.json())
    }
    const applyTip = async () => {
      const mark = page.getByTestId("tip-mark")
      await expect(mark).toHaveCount(1)
      await mark.click()
      const popover = page.getByTestId("tip-popover")
      await expect(popover).toContainText(`Finish with Zitrone ${tag}.`)
      await expect(popover).toContainText(
        "Something sharp lifts the rich plate."
      )
      await page.getByTestId("tip-apply").click()
      await expect(page.getByText("Tip applied")).toBeVisible()
      await expect(mark).toHaveCount(0)
    }

    await page.goto(`/plan?start=${start}`)
    await applyTip()
    // Nutrition and the shopping list follow (AC-5).
    expect(await shop()).toContain(`Zitrone ${tag}`)
    // Undo takes the lemon off again and the tip comes back.
    await page.getByRole("button", { name: "Undo" }).click()
    await expect(page.getByTestId("tip-mark")).toHaveCount(1)
    expect(await shop()).not.toContain(`Zitrone ${tag}`)

    await applyTip()
    await page.goto(`/cook/${sessionId}`)
    await expect(page.getByTestId("cook-finish")).toContainText(
      `Zitrone ${tag}, 10 g`
    )
  } finally {
    await ctx.dispose()
    await cleanupFood(main.id)
    await cleanupFood(lemon.id)
    await cleanupFood(cream.id)
    await cleanupFood(pasta.id)
    await cleanupSlot(slot.id)
  }
})
