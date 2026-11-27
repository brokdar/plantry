import { readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

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
import { SITE } from "./discovery-fixture"

/**
 * C3: with the Chef on, generating a window places a ready find (FIND-3);
 * the plan page lists it under "What the assistant did" and Undo takes it
 * out again (PRO-5). The find comes from a requested run with the fake
 * provider and the fixture site. chef.*, chef.new_dishes_per_week and
 * ai.fake_script are process-global, so this runs in global-settings.
 */

const LEAVES = [
  ["Chicken thighs", 200, 25, 10, 0],
  ["Yellow onion", 40, 1, 0.1, 9],
  ["Basmati rice", 350, 8, 1, 78],
  ["Butter", 717, 1, 81, 0.1],
  ["Sea salt", 0, 0, 0, 0],
] as const

async function clearGlobals() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  for (const key of [
    "chef.enabled",
    "chef.sites",
    "ai.fake_script",
    "chef.new_dishes_per_week",
  ]) {
    await ctx.delete(`/api/settings/${key}`)
  }
  await ctx.dispose()
}

test.afterEach(clearGlobals)

test.setTimeout(180_000)

test("generating places a ready find; the list shows it and Undo removes it", async ({
  page,
}) => {
  const tag = uid()
  const foods: number[] = []
  const leafIds: number[] = []
  for (const [name, kcal, protein, fat, carbs] of LEAVES) {
    const f = await seedLeafFood({
      name: `${name} ${tag}`,
      kcal_100g: kcal,
      protein_100g: protein,
      fat_100g: fat,
      carbs_100g: carbs,
    })
    leafIds.push(f.id)
  }
  const { start, days } = await futureWindow()
  const slot = await seedSlot(`slot.rf-${tag}`, "Moon", 99981)
  const base = await seedLeafFood({
    name: `Beans ${tag}`,
    kcal_100g: 120,
    protein_100g: 8,
    fat_100g: 1,
    carbs_100g: 20,
  })
  foods.push(base.id)
  const recipes = [
    ...["Chili", "Curry", "Dal", "Ragout", "Hash", "Pilaf", "Stew"].map(
      (n) => [n, "main"] as const
    ),
    ...["Rice", "Bread"].map((n) => [n, "side_starch"] as const),
    ...["Greens", "Slaw"].map((n) => [n, "side_veg"] as const),
  ]
  for (const [dish, role] of recipes) {
    const f = await seedComposedFood({
      name: `${dish} ${tag}`,
      role,
      reference_portions: 2,
      keep_days: 2,
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
  }
  const script = join(tmpdir(), `plantry-chef-ready-${tag}.json`)
  const raw = await readFile(
    join(process.cwd(), "e2e/fixtures/chat-scripts/chef-find.json"),
    "utf8"
  )
  await writeFile(
    script,
    raw.replaceAll("{{SITE}}", SITE).replaceAll("{{RUN}}", tag)
  )
  const ctx = await apiRequest.newContext({ baseURL: API })
  try {
    await setSetting("ai.fake_script", script)
    await setSetting(
      "chef.sites",
      JSON.stringify([
        {
          domain: new URL(SITE).hostname,
          lang: "en",
          enabled: true,
          ai_groups_exempt: false,
        },
      ])
    )
    await setSetting("chef.enabled", "true")

    const find = await ctx.post("/api/finds", {
      data: { dish: "chicken curry" },
    })
    expect(find.ok(), await find.text()).toBeTruthy()
    const { id: findId } = (await find.json()) as { id: number }
    await expect
      .poll(
        async () =>
          (
            (await (await ctx.get(`/api/finds/${findId}`)).json()) as {
              state: string
            }
          ).state,
        { timeout: 60_000 }
      )
      .toBe("ready")

    await setSetting("chef.new_dishes_per_week", "1")
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
            (await (await ctx.get(`/api/plan/runs/${run_id}`)).json()) as {
              status: string
            }
          ).status
      )
      .toBe("ok")

    await page.goto(`/plan?start=${start}`)
    const list = page.getByTestId("assistant-did")
    await expect(list).toContainText("Added Mild chicken curry to this week")
    await expect(page.getByText("Mild chicken curry").first()).toBeVisible()

    await list.getByRole("button", { name: /Undo/ }).click()
    await expect(list).toHaveCount(0)
    const after = (await (await ctx.get(`/api/finds/${findId}`)).json()) as {
      state: string
    }
    expect(after.state).toBe("ready")
  } finally {
    await ctx.delete(`/api/plan?from=${start}&to=${days[6]}`)
    await ctx.dispose()
    await cleanupSlot(slot.id)
    for (const id of foods) await cleanupFood(id)
    for (const id of leafIds) await cleanupFood(id)
    await rm(script, { force: true })
  }
})
