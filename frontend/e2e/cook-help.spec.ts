import { rmSync, writeFileSync } from "node:fs"
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

/**
 * Cook-mode Help (HELP-1, HELP-2) at phone width through the fake provider:
 * three tiles, the model tiles answer in at most three lines, "I don't have
 * an ingredient" stays the instant rule-based flow. chef.enabled and the fake
 * script are process-global, so this spec runs in global-settings and clears
 * what it writes.
 */

const NUTRITION = {
  kcal_100g: 40,
  protein_100g: 1.1,
  fat_100g: 0.1,
  carbs_100g: 9.3,
}

async function clearSettings() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  for (const key of ["chef.enabled", "ai.fake_script"]) {
    await ctx.delete(`/api/settings/${key}`)
  }
  await ctx.dispose()
}

test.afterEach(clearSettings)

const helpTurn = (lines: string[]) => ({
  for_tool: "chef_help_answer",
  stop_reason: "tool_use",
  message: {
    content: [
      {
        type: "tool_use",
        tool_use_id: "toolu_help",
        tool_name: "chef_help_answer",
        tool_input: { lines },
      },
    ],
  },
  usage: { input_tokens: 700, output_tokens: 60 },
})

interface Seed {
  sessionId: number
  chiliId: number
  onion: string
  cleanup: () => Promise<void>
}

async function seed(): Promise<Seed> {
  const tag = uid()
  const { days } = await futureWindow()
  const slot = await seedSlot(`slot.help-${tag}`, "Moon", 1_000_000)
  const beans = await seedLeafFood({ name: `Kidney beans (${tag})` })
  const onion = await seedLeafFood({
    name: `Zwiebel (${tag})`,
    ...NUTRITION,
  })
  const shallot = await seedLeafFood({
    name: `Schalotte (${tag})`,
    ...NUTRITION,
  })
  const chili = await seedComposedFood({
    name: `Chili ${tag}`,
    role: "main",
    reference_portions: 4,
    children: [
      { child_id: beans.id, amount: 400, unit: "g", grams: 400, sort_order: 0 },
      {
        child_id: onion.id,
        amount: 2,
        unit: "piece",
        grams: 240,
        sort_order: 1,
      },
    ],
    instructions: [{ step_number: 1, text: "Chop the onions." }],
  })
  const ctx = await apiRequest.newContext({ baseURL: API })
  const plate = await ctx.post("/api/plates", {
    data: {
      date: days[1],
      slot_id: slot.id,
      components: [{ food_id: chili.id, portions: 1 }],
    },
  })
  expect(plate.ok(), await plate.text()).toBeTruthy()
  const body = (await plate.json()) as {
    components: { cook_session_id: number }[]
  }
  const sessionId = body.components[0].cook_session_id
  const put = await ctx.put(`/api/cook-sessions/${sessionId}`, {
    data: { portions: 6, frozen_portions: 0, note: null },
  })
  expect(put.ok(), await put.text()).toBeTruthy()
  await ctx.dispose()
  return {
    sessionId,
    chiliId: chili.id,
    onion: onion.name,
    cleanup: async () => {
      await cleanupSlot(slot.id)
      await cleanupFood(chili.id)
      await cleanupFood(beans.id)
      await cleanupFood(onion.id)
      await cleanupFood(shallot.id)
    },
  }
}

test("the three tiles: explain a step, something went wrong, no ingredient", async ({
  page,
}) => {
  const s = await seed()
  const scriptPath = join(tmpdir(), `plantry-cook-help-${uid()}.json`)
  writeFileSync(
    scriptPath,
    JSON.stringify({
      turns: [
        // Every request starts the forced tool's sequence again, so both
        // model tiles get this turn.
        helpTurn(["Chop the onions small.", "Even pieces cook evenly."]),
      ],
    })
  )
  try {
    await setSetting("ai.fake_script", scriptPath)
    await setSetting("chef.enabled", "true")
    await page.setViewportSize({ width: 375, height: 812 })
    await page.goto(`/cook/${s.sessionId}`)

    const open = page.getByTestId("help-open")
    await expect(open).toBeVisible()
    expect((await open.boundingBox())?.height).toBeGreaterThanOrEqual(56)
    await expect(page.getByTestId("substitute-open")).toHaveCount(0)

    // Explain this step.
    await open.click()
    await expect(page.getByTestId("help-tile-substitute")).toBeVisible()
    await page.getByTestId("help-tile-explain").click()
    const answer = page.getByTestId("help-answer")
    await expect(answer).toContainText("Chop the onions small.")
    await expect(answer.locator("li")).toHaveCount(2)
    // The one-tap verdict is kept and changeable.
    await page.getByTestId("help-verdict-helped").click()
    await expect(page.getByTestId("help-verdict-helped")).toHaveAttribute(
      "aria-pressed",
      "true"
    )
    await page.getByTestId("help-verdict-not_helped").click()
    await expect(page.getByTestId("help-verdict-not_helped")).toHaveAttribute(
      "aria-pressed",
      "true"
    )
    await page.keyboard.press("Escape")

    // Something went wrong: a chip, then the answer.
    await open.click()
    await page.getByTestId("help-tile-wrong").click()
    await page.getByTestId("help-problem-too_salty").click()
    await page.getByTestId("help-wrong-send").click()
    await expect(page.getByTestId("help-answer")).toContainText(
      "Chop the onions small."
    )
    await page.keyboard.press("Escape")

    // I don't have an ingredient: the instant rule-based flow, no model call.
    await open.click()
    await page.getByTestId("help-tile-substitute").click()
    const dialog = page.getByTestId("substitute-dialog")
    await dialog
      .getByTestId("substitute-pick")
      .filter({ hasText: "Zwiebel" })
      .click()
    await expect(dialog.getByTestId("substitute-option").first()).toContainText(
      /360 g .*Schalotte/
    )
  } finally {
    rmSync(scriptPath, { force: true })
    await s.cleanup()
  }
})
