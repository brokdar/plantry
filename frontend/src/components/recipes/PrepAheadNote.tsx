import { useTranslation } from "react-i18next"

import type { PrepAhead } from "@/lib/api/foods"

/** The "Start the day before: marinate" note of a recipe whose steps need a
 *  step before the cook day; "Start 4 h ahead" when a step states hours. */
export function PrepAheadNote({
  prepAhead,
  className = "",
}: {
  prepAhead?: PrepAhead | null
  className?: string
}) {
  const { t } = useTranslation()
  if (!prepAhead) return null
  const what = t(`recipes.prep_ahead.${prepAhead.kind}`)
  return (
    <span data-testid="prep-ahead" className={className}>
      {prepAhead.hours
        ? t("recipes.prep_ahead.hours", { count: prepAhead.hours, what })
        : t("recipes.prep_ahead.day_before", { what })}
    </span>
  )
}
