import { rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"

import {
  API,
  apiRequest,
  cleanupFood,
  expect,
  mockSettingValues,
  seedComposedFood,
  seedLeafFood,
  setSetting,
  test,
  uid,
} from "./helpers"

/**
 * chef.enabled is process-global, so this spec runs in the global-settings
 * project and clears the override it writes. The provider is page-mocked:
 * the switch shows only with a usable provider (ON-1).
 */

async function clearChefEnabled() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete("/api/settings/chef.enabled")
  await ctx.dispose()
}

test.afterEach(clearChefEnabled)

test("the Chef switch turns on with a provider and shows this month's spend", async ({
  page,
}) => {
  await clearChefEnabled()
  await mockSettingValues(page, { "ai.provider": "fake" })
  await page.goto("/settings/assistant")

  const toggle = page.getByLabel("Chef", { exact: true })
  await expect(toggle).not.toBeChecked()
  await expect(page.getByTestId("chef-spend")).toHaveText(
    /^This month: \$\d+\.\d{2,4} of \$5\.00/
  )

  // ON-2: the labelling estimate first; the switch saves on Turn on.
  await toggle.click()
  await expect(page.getByTestId("chef-on-estimate")).not.toHaveText(
    "Working out the cost…"
  )
  await page.getByRole("button", { name: "Turn on" }).click()
  await expect(page.getByTestId("chef-on-dialog")).toHaveCount(0)
  await expect(toggle).toBeChecked()
  await expect(
    page.getByText("The assistant labels your recipes", { exact: false })
  ).toBeVisible()

  const ctx = await apiRequest.newContext({ baseURL: API })
  const body = (await (await ctx.get("/api/settings")).json()) as {
    items: { key: string; value?: string }[]
  }
  await ctx.dispose()
  expect(body.items.find((i) => i.key === "chef.enabled")?.value).toBe("true")
  // The save's settings refetch may still be in the mocked route.
  await page.unrouteAll({ behavior: "ignoreErrors" })
})

test("the Chef switch is hidden without a provider", async ({ page }) => {
  await mockSettingValues(page, { "ai.provider": "" })
  await page.goto("/settings/assistant")
  await expect(page.getByLabel("Daily suggestion")).toBeVisible()
  await expect(page.getByLabel("Chef", { exact: true })).toHaveCount(0)
})

/** A set_recipe_attributes reply for every key of a full batch. */
function labelScript(): string {
  const recipes = Array.from({ length: 8 }, (_, i) => ({
    key: `r${i + 1}`,
    kind: "stew",
    technique: "simmer",
  }))
  return JSON.stringify({
    turns: [
      {
        for_tool: "set_recipe_attributes",
        stop_reason: "tool_use",
        message: {
          content: [
            {
              type: "tool_use",
              tool_use_id: "toolu_label",
              tool_name: "set_recipe_attributes",
              tool_input: { recipes },
            },
          ],
        },
        usage: { input_tokens: 1500, output_tokens: 400 },
      },
    ],
  })
}

test("turning the Chef on labels the library: the estimate, then chips filled by the assistant", async ({
  page,
}) => {
  await clearChefEnabled()
  const tag = uid()
  const scriptPath = join(tmpdir(), `plantry-labels-${tag}.json`)
  writeFileSync(scriptPath, labelScript())
  const leaf = await seedLeafFood({
    name: `Linsen ${tag}`,
    kcal_100g: 116,
    protein_100g: 9,
    fat_100g: 0.4,
    carbs_100g: 20,
  })
  const recipe = await seedComposedFood({
    name: `Linseneintopf ${tag}`,
    role: "main",
    children: [
      { child_id: leaf.id, amount: 300, unit: "g", grams: 300, sort_order: 0 },
    ],
  })
  try {
    // Autonomous work needs a priced model (COST-3).
    await setSetting(
      "ai.price_overrides",
      JSON.stringify({ "fake-e2e": { in: 3, out: 15 } })
    )
    await setSetting("ai.fake_script", scriptPath)

    await page.goto("/settings/assistant")
    await page.getByLabel("Chef", { exact: true }).click()
    await expect(page.getByTestId("chef-on-estimate")).toContainText(
      /recipes?, about \$\d+\.\d+ and \d+ min\. It runs in the background/
    )
    const started = page.waitForResponse(
      (r) =>
        r.url().endsWith("/api/chef/labelling") &&
        r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "Turn on" }).click()
    expect((await started).status()).toBe(202)

    // The queued jobs run in the background through the fake provider.
    const ctx = await apiRequest.newContext({ baseURL: API })
    await expect
      .poll(async () => {
        const res = await ctx.get(`/api/foods/${recipe.id}/attributes`)
        return ((await res.json()) as { sources: Record<string, string> })
          .sources.kind
      })
      .toBe("auto")
    await ctx.dispose()

    await page.goto(`/recipes/${recipe.id}`)
    const kind = page.getByTestId("attr-chip-kind")
    await expect(kind).toContainText("Stew")
    await expect(kind).toHaveAttribute("data-source", "auto")
  } finally {
    const ctx = await apiRequest.newContext({ baseURL: API })
    await ctx.delete("/api/settings/ai.fake_script")
    await ctx.delete("/api/settings/ai.price_overrides")
    await ctx.dispose()
    await clearChefEnabled()
    await cleanupFood(recipe.id)
    await cleanupFood(leaf.id)
    rmSync(scriptPath, { force: true })
  }
})
