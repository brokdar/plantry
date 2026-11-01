import type { Plan } from "@/lib/api/plan"
import type { Session } from "@/lib/api/sessions"
import { formatDate, type Formatter } from "@/lib/plan/format"

/** A suggestion: `text` fills the composer. */
export interface Chip {
  id: string
  text: string
}

/** The draft session the freeze chip would be about. */
export function freezeCandidate(plan: Plan): Session | undefined {
  return plan.sessions.find((s) => s.state === "draft")
}

/**
 * Suggestion chips for an empty conversation (D18), from the plan read model
 * alone: two fixed ones, "Make the {name} {portions + 2} portions, freeze 2"
 * for a draft session of a freezable recipe while freezing is on, and
 * "Different side on {weekday}" for the first draft plate with a side.
 * `freezable` is the candidate recipe's flag (`freezeCandidate`).
 */
export function buildChips(
  plan: Plan,
  opts: { freezing: boolean; freezable: boolean },
  f: Formatter
): Chip[] {
  const chips: Chip[] = []
  const session = freezeCandidate(plan)
  if (session && opts.freezing && opts.freezable) {
    chips.push({
      id: "freeze",
      text: f.t("assistant.chips.freeze", {
        name: session.food_name,
        portions: session.portions + 2,
      }),
    })
  }
  const withSide = plan.cells
    .map((c) => c.plate)
    .filter((p) => p?.state === "draft")
    .sort((a, b) => a!.date.localeCompare(b!.date))
    .find((p) => p!.components.some((c) => c.role?.startsWith("side_")))
  if (withSide) {
    chips.push({
      id: "side",
      text: f.t("assistant.chips.side", {
        weekday: formatDate(withSide.date, f, { weekday: "long" }),
      }),
    })
  }
  chips.push(
    { id: "lighter", text: f.t("assistant.chips.lighter") },
    { id: "no_fish", text: f.t("assistant.chips.no_fish") }
  )
  return chips
}
