import { rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

import { request as apiRequest } from "@playwright/test"

import {
  API,
  cleanupFood,
  expect,
  mockSettingValues,
  seedComposedFood,
  seedLeafFood,
  test,
  uid,
} from "./helpers"

// "Ask about this recipe" (Chef V1): the recipe page asks the fake provider,
// whose scripted chef_recipe_change turn is routed by its forced tool. Serial
// (ai-serial): the fake script is a global setting.
const letters = (n: string) => n.replace(/\d/g, (d) => "abcdefghij"[Number(d)])

function turn(input: object): string {
  return JSON.stringify({
    turns: [
      {
        for_tool: "chef_recipe_change",
        stop_reason: "tool_use",
        message: {
          content: [
            {
              type: "tool_use",
              tool_use_id: "toolu_rc",
              tool_name: "chef_recipe_change",
              tool_input: input,
            },
          ],
        },
        usage: { input_tokens: 800, output_tokens: 120 },
      },
    ],
  })
}

async function seedRecipe(tag: string) {
  const beans = await seedLeafFood({
    name: `Beans${tag}`,
    kcal_100g: 120,
    protein_100g: 8,
    fat_100g: 1,
    carbs_100g: 18,
  })
  const yoghurt = await seedLeafFood({
    name: `Yoghurt${tag}`,
    kcal_100g: 110,
    protein_100g: 4,
    fat_100g: 5,
    carbs_100g: 6,
  })
  const recipe = await seedComposedFood({
    name: `Stew${tag}`,
    role: "main",
    children: [
      { child_id: beans.id, amount: 300, unit: "g", grams: 300, sort_order: 0 },
    ],
    instructions: [{ step_number: 1, text: "Simmer for 20 minutes." }],
  })
  return { beans, yoghurt, recipe }
}

async function useScript(script: string, name: string) {
  const path = join(tmpdir(), name)
  await writeFile(path, script)
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.put("/api/settings/ai.fake_script", {
    data: { value: path },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  await ctx.dispose()
  return path
}

test("Make a variant saves a variant with its changes list, and Undo removes it", async ({
  page,
}) => {
  const tag = letters(uid())
  const { beans, yoghurt, recipe } = await seedRecipe(tag)
  const path = await useScript(
    turn({
      kind: "variant",
      recipe: {
        name: `Creamy stew ${tag}`,
        ingredients: [`300 g Beans${tag}`, `100 g Yoghurt${tag}`],
        steps: ["Simmer for 20 minutes.", "Stir in the yoghurt."],
        changes: ["Yoghurt makes it creamy."],
      },
    }),
    `plantry-recipe-chat-${tag}-a.json`
  )
  try {
    await mockSettingValues(page, {
      "chef.enabled": "true",
      "ai.provider": "fake",
    })
    await page.goto(`/recipes/${recipe.id}`)
    await page.getByTestId("ask-chip-variant").click()
    const saved = page.getByTestId("recipe-chat-variant")
    await expect(saved).toContainText("Yoghurt makes it creamy.")
    await expect(saved.getByRole("link")).toHaveText(`Creamy stew ${tag}`)

    const ctx = await apiRequest.newContext({ baseURL: API })
    const variantNames = async () =>
      (
        (await (await ctx.get("/api/foods?limit=500")).json()) as {
          items: { name: string }[]
        }
      ).items.map((i) => i.name)
    expect(await variantNames()).toContain(`Creamy stew ${tag}`)

    // Undo from the toast removes the variant.
    await page.getByRole("button", { name: "Undo" }).click()
    await expect
      .poll(async () => await variantNames())
      .not.toContain(`Creamy stew ${tag}`)
    await ctx.dispose()
    await cleanupFood(yoghurt.id).catch(() => {})
  } finally {
    await page.unrouteAll({ behavior: "ignoreErrors" })
    await rm(path, { force: true })
    await cleanupFood(recipe.id)
    await cleanupFood(beans.id).catch(() => {})
  }
})

test("a variant that breaks a rule is not saved and says why", async ({
  page,
}) => {
  const tag = letters(uid())
  const { beans, recipe } = await seedRecipe(tag)
  const path = await useScript(
    turn({
      kind: "variant",
      recipe: {
        name: `Mystery stew ${tag}`,
        ingredients: [`300 g Beans${tag}`, `2 tbsp Unobtainium${tag}`],
        steps: ["Simmer for 20 minutes."],
        changes: ["Adds a mystery."],
      },
    }),
    `plantry-recipe-chat-${tag}-b.json`
  )
  try {
    await mockSettingValues(page, {
      "chef.enabled": "true",
      "ai.provider": "fake",
    })
    await page.goto(`/recipes/${recipe.id}`)
    await page.getByTestId("ask-input").fill("add something odd")
    await page.getByTestId("ask-input").press("Enter")
    await expect(page.getByTestId("recipe-chat-blocked")).toBeVisible()
    await expect(page.getByTestId("recipe-check-block").first()).toBeVisible()
    await expect(page.getByTestId("recipe-chat-variant")).toHaveCount(0)
  } finally {
    await page.unrouteAll({ behavior: "ignoreErrors" })
    await rm(path, { force: true })
    await cleanupFood(recipe.id)
    await cleanupFood(beans.id).catch(() => {})
  }
})

test("Check this recipe needs no model and the entry is hidden with Chef off", async ({
  page,
}) => {
  const tag = letters(uid())
  const { beans, recipe } = await seedRecipe(tag)
  try {
    await mockSettingValues(page, {
      "chef.enabled": "true",
      "ai.provider": "fake",
    })
    await page.goto(`/recipes/${recipe.id}`)
    await page.getByTestId("ask-chip-check").click()
    await expect(page.getByTestId("recipe-check")).toBeVisible()
    await page.unrouteAll({ behavior: "ignoreErrors" })

    await mockSettingValues(page, { "chef.enabled": "false" })
    await page.goto(`/recipes/${recipe.id}`)
    await expect(page.getByTestId("recipe-attributes")).toBeVisible()
    await expect(page.getByTestId("ask-recipe")).toHaveCount(0)
  } finally {
    await page.unrouteAll({ behavior: "ignoreErrors" })
    await cleanupFood(recipe.id)
    await cleanupFood(beans.id).catch(() => {})
  }
})
