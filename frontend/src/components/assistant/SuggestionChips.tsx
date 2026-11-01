import { useTranslation } from "react-i18next"

import { buildChips, freezeCandidate } from "@/lib/assistant/chips"
import type { Plan } from "@/lib/api/plan"
import { useFood } from "@/lib/queries/foods"
import { useSettingItems } from "@/lib/queries/settings"

interface SuggestionChipsProps {
  plan: Plan
  /** A chip fills the composer; it never sends. */
  onPick: (text: string) => void
}

/** Starting points for an empty conversation, from the plan (D18). */
export function SuggestionChips({ plan, onPick }: SuggestionChipsProps) {
  const { t, i18n } = useTranslation()
  const settings = useSettingItems()
  const freezing = settings.data?.["plan.freezing_enabled"]?.value === "true"
  const candidate = freezeCandidate(plan)
  const food = useFood(freezing && candidate ? candidate.food_id : 0)
  const chips = buildChips(
    plan,
    {
      freezing,
      freezable: food.data?.kind === "composed" && food.data.freezable === true,
    },
    i18n
  )

  return (
    <ul
      aria-label={t("assistant.chips.label")}
      data-testid="assistant-chips"
      className="flex flex-wrap gap-2"
    >
      {chips.map((chip) => (
        <li key={chip.id}>
          <button
            type="button"
            onClick={() => onPick(chip.text)}
            className="inline-flex min-h-11 items-center rounded-full border border-line bg-panel px-3 text-small text-ink transition-colors duration-150 hover:bg-panel-sunken md:pointer-fine:min-h-8"
          >
            {chip.text}
          </button>
        </li>
      ))}
    </ul>
  )
}
