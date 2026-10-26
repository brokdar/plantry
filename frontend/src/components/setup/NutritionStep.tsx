import { NutritionGoalFields } from "@/components/settings/ProfileEditor"
import type { NutritionTargets } from "@/lib/nutrition-presets"

/** Cut / maintain / bulk cards plus custom targets (plan D6). */
export function NutritionStep({
  value,
  onChange,
}: {
  value: NutritionTargets
  onChange: (next: NutritionTargets) => void
}) {
  return <NutritionGoalFields value={value} onCommit={onChange} />
}
