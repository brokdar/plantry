import type { TFunction } from "i18next"

import type {
  AvailabilityGridColumn,
  AvailabilityGridValue,
} from "@/components/availability/AvailabilityGrid"
import type { AvailabilityDefault } from "@/lib/api/availability"

const WEEKDAYS = [0, 1, 2, 3, 4, 5, 6] as const

/** Monday..Sunday columns keyed "0".."6" (weekday 0 = Monday), with a short
 *  header label and the full day name for assistive tech. */
export function weekdayColumns(t: TFunction): AvailabilityGridColumn[] {
  return WEEKDAYS.map((d) => ({
    key: String(d),
    label: t(`availability.weekday_short.${d}`),
    fullLabel: t(`availability.weekday_long.${d}`),
  }))
}

/** Server defaults → grid value keyed `"<weekday>:<slotId>"`. */
export function defaultsToGridValue(
  items: AvailabilityDefault[]
): AvailabilityGridValue {
  return Object.fromEntries(
    items.map((item) => [
      `${item.weekday}:${item.slot_id}`,
      item.mode === "routine" && item.preset_id != null
        ? { mode: "routine" as const, presetId: item.preset_id }
        : { mode: item.mode },
    ])
  )
}

/** Grid value → the full weekday × slot defaults grid for
 *  `PUT /api/availability/defaults`. Missing cells are `plan`; `preset_id`
 *  is set only for routines. */
export function gridValueToDefaults(
  value: AvailabilityGridValue,
  slots: { id: number }[]
): AvailabilityDefault[] {
  return WEEKDAYS.flatMap((weekday) =>
    slots.map((slot) => {
      const cell = value[`${weekday}:${slot.id}`] ?? { mode: "plan" }
      return {
        weekday,
        slot_id: slot.id,
        mode: cell.mode,
        preset_id: cell.mode === "routine" ? (cell.presetId ?? null) : null,
      }
    })
  )
}
