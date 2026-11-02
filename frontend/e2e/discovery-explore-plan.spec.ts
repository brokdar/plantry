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

// Phase 5 F4 / AC-4.1: a heuristic generation includes the explore recipe
// exactly once when `plan.explore_per_week = 1`. The recipe is seeded with
// the `explore` tag a saved suggestion gets (discovery.spec.ts walks the
// accepting); this spec is about what the generator does with it. It writes
// a global plan setting and fills every slot of a window, so it runs alone
// in `global-settings`.

interface RunDetail {
  status: string
  score: { variety: { explore: { target: number; placed: number } } } | null
}

test("F4-AC4: with one explore recipe a week and quota 1, the generated window has it exactly once", async () => {
  const tag = uid()
  const { start, days } = await futureWindow()
  const slot = await seedSlot(`slot.dep-${tag}`, "Moon", 99982)
  const base = await seedLeafFood({
    name: `Beans ${tag}`,
    kcal_100g: 120,
    protein_100g: 8,
    fat_100g: 1,
    carbs_100g: 20,
  })
  const foods = [base.id]
  const recipes = [
    ...[
      "Chili",
      "Curry",
      "Dal",
      "Ragout",
      "Hash",
      "Pilaf",
      "Stew",
      "Gratin",
    ].map((n) => [n, "main", []] as const),
    ["Fesenjan", "main", ["explore"]] as const,
    ...["Rice", "Bread"].map((n) => [n, "side_starch", []] as const),
    ...["Greens", "Slaw"].map((n) => [n, "side_veg", []] as const),
  ]
  let fesenjan = 0
  for (const [dish, role, tags] of recipes) {
    const f = await seedComposedFood({
      name: `${dish} ${tag}`,
      role,
      reference_portions: 2,
      keep_days: 2,
      tags: [...tags],
      children: [
        {
          child_id: base.id,
          amount: 300,
          unit: "g",
          grams: 300,
          sort_order: 0,
        },
      ],
    })
    foods.unshift(f.id)
    if (dish === "Fesenjan") fesenjan = f.id
  }
  const ctx = await apiRequest.newContext({ baseURL: API })
  try {
    await setSetting("plan.explore_per_week", "1")
    await ctx.delete(`/api/plan?from=${start}&to=${days[6]}`)
    const res = await ctx.post("/api/plan/generate", {
      data: { start, chooser: "heuristic" },
    })
    expect(res.status(), await res.text()).toBe(202)
    const { run_id } = (await res.json()) as { run_id: number }
    await expect
      .poll(
        async () =>
          (
            (await (
              await ctx.get(`/api/plan/runs/${run_id}`)
            ).json()) as RunDetail
          ).status
      )
      .toBe("ok")

    const plan = (await (await ctx.get(`/api/plan?start=${start}`)).json()) as {
      cells: {
        plate: {
          components: { food_id: number; cook_session_id: number | null }[]
        } | null
      }[]
    }
    const placed = plan.cells
      .flatMap((c) => c.plate?.components ?? [])
      .filter((c) => c.food_id === fesenjan)
    // A cook batch spreads over several plates: it is one cooking.
    expect(placed.length).toBeGreaterThan(0)
    expect(new Set(placed.map((c) => c.cook_session_id)).size).toBe(1)

    const run = (await (
      await ctx.get(`/api/plan/runs/${run_id}`)
    ).json()) as RunDetail
    expect(run.score?.variety.explore).toEqual({ target: 1, placed: 1 })
  } finally {
    await ctx.delete("/api/settings/plan.explore_per_week")
    await ctx.delete(`/api/plan?from=${start}&to=${days[6]}`)
    await ctx.dispose()
    await cleanupSlot(slot.id)
    for (const id of foods) await cleanupFood(id)
  }
})
