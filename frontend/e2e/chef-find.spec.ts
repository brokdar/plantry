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
 * C2: a requested find with the Chef on, end to end with the fake provider
 * and the fixture site. Discover takes a dish, the pipeline fetches the
 * page, adapts it in Plantry's words and the review card shows the source
 * and what changed; "Add to my recipes" opens the import review and keep
 * saves it. Then, on a phone, cook mode shows the recipe with no image,
 * the adapted steps and the seasoning as "to taste".
 * chef.enabled, chef.sites and ai.fake_script are process-global, so this
 * runs in global-settings and clears what it writes.
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

test.setTimeout(120_000)

test("a requested find: review card, keep, then cook it on a phone", async ({
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
  const script = join(tmpdir(), `plantry-chef-find-${tag}.json`)
  const raw = await readFile(
    join(process.cwd(), "e2e/fixtures/chat-scripts/chef-find.json"),
    "utf8"
  )
  await writeFile(
    script,
    raw.replaceAll("{{SITE}}", SITE).replaceAll("{{RUN}}", tag)
  )
  let foodId: number | null = null
  let slot: { id: number } | null = null
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
    await page.getByTestId("add-recipe").click()
    await page.getByRole("menuitem", { name: "Discover recipes" }).click()
    await page.getByTestId("discover-dish").fill("chicken curry")
    await page.getByTestId("discover-find").click()

    // The state first, so a failed run says why.
    await expect
      .poll(
        async () => {
          const res = await page.request.get(`${API}/api/finds`)
          const { items } = (await res.json()) as {
            items: { state: string; steps: unknown }[]
          }
          return items
            .map((f) => `${f.state} ${JSON.stringify(f.steps)}`)
            .join(",")
        },
        { timeout: 45_000 }
      )
      .toMatch(/ready/)
    const review = page.getByTestId("find-review")
    await expect(review).toBeVisible()
    await expect(
      review.getByRole("heading", { name: "Mild chicken curry" })
    ).toBeVisible()
    await expect(page.getByTestId("find-source")).toContainText(
      new URL(SITE).hostname
    )

    await page.getByTestId("find-add").click()
    await page.getByRole("button", { name: "Save recipe" }).click()
    await page.waitForURL(/\/recipes\/\d+/, { timeout: 15_000 })
    foodId = Number(new URL(page.url()).pathname.split("/").pop())

    // Cook it on a phone: a plate gives the cook route.
    const { days } = await futureWindow()
    slot = await seedSlot(`slot.find-${tag}`, "Moon", 1_200_000)
    const ctx = await apiRequest.newContext({ baseURL: API })
    const plateRes = await ctx.post("/api/plates", {
      data: {
        date: days[1],
        slot_id: slot.id,
        components: [{ food_id: foodId, portions: 1 }],
      },
    })
    expect(plateRes.ok(), await plateRes.text()).toBeTruthy()
    const plate = (await plateRes.json()) as {
      components: { cook_session_id: number }[]
    }
    await ctx.dispose()

    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`/cook/${plate.components[0].cook_session_id}`)
    await expect(
      page.getByRole("heading", { level: 1, name: "Mild chicken curry" })
    ).toBeVisible()
    await expect(page.locator("img")).toHaveCount(0)
    await expect(
      page.getByTestId("cook-ingredient").filter({ hasText: "Sea salt" })
    ).toContainText("to taste")
    await page.getByRole("tab", { name: "Steps" }).click()
    await expect(
      page.getByText("Sear the chicken thighs in the butter until golden.")
    ).toBeVisible()
  } finally {
    if (slot) await cleanupSlot(slot.id)
    if (foodId !== null) await cleanupFood(foodId)
    for (const id of leafIds) await cleanupFood(id)
    await rm(script, { force: true })
  }
})
