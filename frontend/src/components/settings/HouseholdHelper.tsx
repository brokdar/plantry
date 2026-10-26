import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Stepper } from "./controls"
import {
  MAX_HOUSEHOLD_PORTIONS,
  householdCounts,
  householdPortions,
} from "./household"

/**
 * Adults and children steppers that compute `household_portions`. Controlled
 * by the stored portions; the counts are kept locally from the first value,
 * because 1 adult + 2 children and 2 adults store the same number.
 * `onChange` receives the new portions.
 */
export function HouseholdHelper({
  value,
  onChange,
}: {
  value: number
  onChange: (portions: number) => void
}) {
  const { t } = useTranslation()
  const [counts, setCounts] = useState(() => householdCounts(value))

  function update(next: { adults: number; children: number }) {
    const portions = householdPortions(next.adults, next.children)
    if (portions <= 0 || portions > MAX_HOUSEHOLD_PORTIONS) return
    setCounts(next)
    onChange(portions)
  }

  const adultsLabel = t("settings_page.household.adults")
  const childrenLabel = t("settings_page.household.children")

  return (
    <div className="flex flex-wrap gap-x-8 gap-y-3">
      <div className="flex flex-col gap-1">
        <span className="text-small text-ink-muted">{adultsLabel}</span>
        <Stepper
          label={adultsLabel}
          value={counts.adults}
          min={counts.children > 0 ? 0 : 1}
          max={MAX_HOUSEHOLD_PORTIONS}
          onChange={(adults) => update({ ...counts, adults })}
        />
      </div>
      <div className="flex flex-col gap-1">
        <span className="text-small text-ink-muted">{childrenLabel}</span>
        <Stepper
          label={childrenLabel}
          value={counts.children}
          min={counts.adults > 0 ? 0 : 1}
          max={MAX_HOUSEHOLD_PORTIONS * 2}
          onChange={(children) => update({ ...counts, children })}
        />
      </div>
    </div>
  )
}
