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
  seedLeafFood,
  seedSlot,
  setSetting,
  test,
  uid,
} from "./helpers"
import { SITE } from "./discovery-fixture"

/**
 * C4: Fill my library and the To try badge, and the cold-start question,
 * end to end with the fake provider and the fixture site. One tap starts ten
 * finds; each ready one enters the library on trial with the badge on its
 * card and on its plate; opening it clears the badge. The cold-start answer
 * proposes chips and a tap stores the item. chef.enabled, chef.sites and
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
  for (const key of ["chef.enabled", "chef.sites", "ai.fake_script"]) {
    await ctx.delete(`/api/settings/${key}`)
  }
  await ctx.dispose()
}

test.afterEach(clearGlobals)
test.setTimeout(150_000)

test("Fill my library: recipes arrive on trial with the badge until opened", async ({
  page,
}) => {
  const tag = uid()
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
  const script = join(tmpdir(), `plantry-chef-trial-${tag}.json`)
  const raw = await readFile(
    join(process.cwd(), "e2e/fixtures/chat-scripts/chef-find.json"),
    "utf8"
  )
  await writeFile(
    script,
    raw.replaceAll("{{SITE}}", SITE).replaceAll("{{RUN}}", tag)
  )
  const foodIds: number[] = []
  let slot: { id: number } | null = null
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

    await page.goto("/recipes")
    await page.getByTestId("fill-library-button").click()
    await expect(page.getByTestId("fill-library-button")).toBeDisabled()

    await expect
      .poll(
        async () => {
          const res = await ctx.get("/api/finds?state=in_library")
          const { items } = (await res.json()) as {
            items: { food_id: number }[]
          }
          foodIds.splice(0, foodIds.length, ...items.map((f) => f.food_id))
          return items.length
        },
        { timeout: 90_000 }
      )
      .toBeGreaterThan(0)
    const foodId = foodIds[0]
    const food = (await (await ctx.get(`/api/foods/${foodId}`)).json()) as {
      trial: boolean
      to_try: boolean
    }
    expect(food).toMatchObject({ trial: true, to_try: true })

    // The badge on the plate.
    const { days } = await futureWindow()
    slot = await seedSlot(`slot.trial-${tag}`, "Moon", 1_200_000)
    const plateRes = await ctx.post("/api/plates", {
      data: {
        date: days[1],
        slot_id: slot.id,
        components: [{ food_id: foodId, portions: 1 }],
      },
    })
    expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
    const plate = (await plateRes.json()) as {
      components: { to_try?: boolean }[]
    }
    expect(plate.components[0].to_try).toBe(true)

    // The badge on the library card, cleared by opening the recipe.
    await page.reload()
    const card = page.getByTestId(`recipe-card-${foodId}`)
    await expect(card.getByTestId("to-try-badge")).toBeVisible()
    await card.click()
    await page.waitForURL(new RegExp(`/recipes/${foodId}`))
    await expect
      .poll(async () => {
        const r = await ctx.get(`/api/foods/${foodId}`)
        return ((await r.json()) as { to_try: boolean }).to_try
      })
      .toBe(false)
    const after = (await (await ctx.get(`/api/foods/${foodId}`)).json()) as {
      trial: boolean
    }
    expect(after.trial, "opening clears the badge, not the trial").toBe(true)
  } finally {
    if (slot) await cleanupSlot(slot.id)
    for (const id of foodIds) await cleanupFood(id)
    for (const id of leafIds) await cleanupFood(id)
    await ctx.dispose()
    await rm(script, { force: true })
  }
})

test("cold start: the answer proposes chips, a tap stores the item", async ({
  page,
}) => {
  const ctx = await apiRequest.newContext({ baseURL: API })
  try {
    await setSetting("chef.enabled", "true")
    await page.goto("/settings/taste")
    await page
      .getByTestId("cold-start-text")
      .fill("We love thai food, but no coriander")
    await page.getByTestId("cold-start-submit").click()
    const chips = page.getByTestId("assistant-statement")
    await expect(chips).toHaveCount(2)
    await expect(chips.nth(0)).toContainText("thai")
    await expect(chips.nth(1)).toContainText("coriander")
    await chips.nth(1).getByTestId("assistant-statement-always").click()
    await expect(chips.nth(1)).toHaveAttribute("data-state", "always")
    await page.reload()
    await expect(
      page
        .getByTestId("taste-rather_not")
        .getByTestId("taste-item")
        .filter({ hasText: "coriander" })
    ).toBeVisible()
  } finally {
    const list = (await (await ctx.get("/api/taste")).json()) as {
      items: { id: number; key: string }[]
    }
    for (const i of list.items.filter((x) => x.key === "coriander"))
      await ctx.delete(`/api/taste/items/${i.id}`)
    await ctx.dispose()
  }
})
