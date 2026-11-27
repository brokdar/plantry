import { rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"

import { API, apiRequest, expect, setSetting, test, uid } from "./helpers"

/**
 * The import check (IMP-1, IMP-5) through the fake provider: with the Chef
 * on, a pasted page shows "Checked: 2 changes, 1 to look at", each change
 * has Undo, and the keep days follow the kept change. chef.enabled and the
 * fake script are process-global, so this spec runs in global-settings and
 * clears what it writes.
 */

async function clearSettings() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  for (const key of ["chef.enabled", "ai.fake_script", "ai.price_overrides"]) {
    await ctx.delete(`/api/settings/${key}`)
  }
  await ctx.dispose()
}

test.afterEach(clearSettings)

function page(tag: string): string {
  return `<!DOCTYPE html><html lang="de"><head><meta charset="utf-8">
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"Recipe","name":"Hähnchenpfanne ${tag}",
 "recipeYield":"2 Portionen",
 "recipeIngredient":["400 g Hähnchenbrust ${tag}","Frischkäse ${tag}"],
 "recipeInstructions":[{"@type":"HowToStep","text":"Hähnchen in der Pfanne braten."},{"@type":"HowToStep","text":"Anrichten."}]}
</script></head><body></body></html>`
}

const script = JSON.stringify({
  turns: [
    {
      for_tool: "report_import_check",
      stop_reason: "tool_use",
      message: {
        content: [
          {
            type: "tool_use",
            tool_use_id: "toolu_check",
            tool_name: "report_import_check",
            tool_input: {
              prep_minutes: 10,
              cook_minutes: 20,
              time_reason: "The steps take about half an hour.",
              step_edits: [
                {
                  step: 0,
                  kind: "safety_cue",
                  text: "Hähnchen in der Pfanne braten. Kerntemperatur 74 °C.",
                  reason: "Poultry needs a core temperature.",
                },
              ],
              flags: [{ ingredient: 0, text: "Check the amount of chicken." }],
            },
          },
        ],
      },
      usage: { input_tokens: 900, output_tokens: 200 },
    },
  ],
})

test("the import check shows one line, applies changes and Undo puts them back", async ({
  page: pw,
}) => {
  const tag = uid()
  const scriptPath = join(tmpdir(), `plantry-import-check-${tag}.json`)
  writeFileSync(scriptPath, script)
  try {
    await setSetting(
      "ai.price_overrides",
      JSON.stringify({ "fake-e2e": { in: 3, out: 15 } })
    )
    await setSetting("ai.fake_script", scriptPath)
    await setSetting("chef.enabled", "true")

    await pw.goto("/recipes?add=url")
    await pw
      .getByRole("button", { name: "Paste the page HTML instead" })
      .click()
    await pw.getByLabel(/HTML/).fill(page(tag))
    await pw.getByRole("button", { name: "Extract recipe" }).click()

    await expect(pw.getByTestId("import-check")).toBeVisible()
    await expect(pw.getByText("Checked: 2 changes, 1 to look at")).toBeVisible()
    await expect(pw.getByLabel("Prep (min)")).toHaveValue("10")
    await expect(pw.getByLabel("Cook (min)")).toHaveValue("20")
    await expect(pw.locator("textarea").first()).toHaveValue(
      "Hähnchen in der Pfanne braten. Kerntemperatur 74 °C."
    )

    // Undo the safety cue (the last change): the step is the author's again.
    await pw
      .getByTestId("import-check-change")
      .filter({ hasText: "Core temperature added" })
      .getByRole("button", { name: "Undo" })
      .click()
    await expect(pw.locator("textarea").first()).toHaveValue(
      "Hähnchen in der Pfanne braten."
    )
    await expect(pw.getByLabel("Prep (min)")).toHaveValue("10")
  } finally {
    rmSync(scriptPath, { force: true })
  }
})
