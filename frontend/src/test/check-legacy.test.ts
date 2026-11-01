// Phase 3 F1-AC4: the legacy gate (scripts/check-legacy.sh, D27) allows the
// Insights page and still rejects the removed names. The patterns are read
// from the script itself and matched like its `grep -E`.
import { describe, expect, it } from "vitest"

import script from "../../scripts/check-legacy.sh?raw"

const block = /patterns=\(\n([\s\S]*?)\n\)/.exec(script)![1]
const patterns = [...block.matchAll(/^\s*'(.+)'$/gm)].map(
  (m) => new RegExp(m[1])
)

const flagged = (line: string) => patterns.some((p) => p.test(line))

// Banned names are assembled so this file doesn't trip the gate it tests.
const banned = (...parts: string[]) => parts.join("")

describe("check-legacy.sh (D27)", () => {
  it("reads the pattern list", () => {
    expect(patterns.length).toBeGreaterThan(5)
  })

  it.each([
    'import { InsightsPage } from "@/components/settings/insights/InsightsPage"',
    "export function RunInsights() {}",
    'const key = insightKeys.summary({ bucket: "week" })',
    'createFileRoute("/settings/assistant/insights")',
    'const label = t("nav.generate_plan")',
    't("plan.review.regenerate_week")',
  ])("allows %s", (line) => {
    expect(flagged(line)).toBe(false)
  })

  it.each([
    banned("const q = use", "Insights()"),
    banned('apiFetch("/foods/', 'insights")'),
    banned('scope: "fill', '_empty"'),
    banned('scope: "replace', '_all"'),
    banned("Most", "Cooked"),
    // Phase 3 Z1 (§0.2 item 5): the chat-prefill generate path and the
    // pre-state lock fields.
    banned('t("plan.generate', '_prompt")'),
    banned('openWith(t("plan.', 'generate_plan"))'),
    banned("plate.locked", "_by"),
    banned('"is_', 'locked": true'),
    banned("const is", "Locked = true"),
  ])("rejects %s", (line) => {
    expect(flagged(line)).toBe(true)
  })
})
