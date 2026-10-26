import { Link } from "@tanstack/react-router"
import { ArrowUpRight } from "lucide-react"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import {
  AvailabilityGrid,
  type AvailabilityGridValue,
} from "@/components/availability/AvailabilityGrid"
import { TimeSlotsEditor } from "@/components/settings/TimeSlotsEditor"
import { Skeleton } from "@/components/ui/skeleton"
import {
  defaultsToGridValue,
  gridValueToDefaults,
  weekdayColumns,
} from "@/lib/availability-grid"
import {
  useAvailabilityDefaults,
  useSetAvailabilityDefaults,
} from "@/lib/queries/availability"
import { usePresets } from "@/lib/queries/presets"
import { useTimeSlots } from "@/lib/queries/slots"
import { slotLabel } from "@/lib/slot-label"

import { SettingsGroup } from "../controls"
import { useSavedNote } from "../setting-field"

const PRESET_PARAMS = { sort: "name", limit: 200 } as const

export function MealsSection() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-10">
      <SettingsGroup
        title={t("settings_page.meals.slots_label")}
        description={t("settings_page.meals.slots_effect")}
      >
        <div className="pt-4">
          <TimeSlotsEditor />
        </div>
      </SettingsGroup>

      <DefaultWeek />

      <SettingsGroup
        title={t("settings_page.meals.routines_label")}
        description={t("settings_page.meals.routines_effect")}
      >
        <div className="pt-4">
          <Link
            to="/presets"
            className="inline-flex min-h-11 items-center gap-1 text-body text-accent underline underline-offset-4 hover:no-underline md:min-h-9"
          >
            {t("settings_page.meals.routines_link")}
            <ArrowUpRight className="size-4" strokeWidth={1.5} aria-hidden />
          </Link>
        </div>
      </SettingsGroup>
    </div>
  )
}

/** The weekday × slot defaults grid, saved through `PUT /defaults` on
 *  every change. */
function DefaultWeek() {
  const { t } = useTranslation()
  const slotsQuery = useTimeSlots(true)
  const defaultsQuery = useAvailabilityDefaults()
  const presetsQuery = usePresets(PRESET_PARAMS)
  const setDefaults = useSetAvailabilityDefaults()
  const { saved, markSaved } = useSavedNote()

  const slots = useMemo(
    () =>
      (slotsQuery.data?.items ?? [])
        .filter((s) => s.active)
        .sort((a, b) => a.sort_order - b.sort_order)
        .map((s) => ({ id: s.id, label: slotLabel(t, s.name_key) })),
    [slotsQuery.data, t]
  )
  const value = useMemo(
    () => defaultsToGridValue(defaultsQuery.data?.items ?? []),
    [defaultsQuery.data]
  )
  const presets = useMemo(
    () =>
      (presetsQuery.data?.items ?? []).map((p) => ({ id: p.id, name: p.name })),
    [presetsQuery.data]
  )

  function save(next: AvailabilityGridValue) {
    setDefaults.mutate(gridValueToDefaults(next, slots), {
      onSuccess: markSaved,
    })
  }

  const loading = slotsQuery.isLoading || defaultsQuery.isLoading
  const label = t("settings_page.meals.grid_label")

  return (
    <SettingsGroup
      title={label}
      description={t("settings_page.meals.grid_effect")}
      saved={saved}
    >
      <div className="pt-4">
        {loading ? (
          <Skeleton className="h-48 w-full" />
        ) : slots.length === 0 ? (
          <p className="text-body text-ink-muted">
            {t("settings_page.meals.grid_no_slots")}
          </p>
        ) : (
          <AvailabilityGrid
            label={label}
            columns={weekdayColumns(t)}
            slots={slots}
            value={value}
            presets={presets}
            surface="ground"
            onChange={(colKey, slotId, cell) =>
              save({ ...value, [`${colKey}:${slotId}`]: cell })
            }
            onBulkChange={(patch) => save({ ...value, ...patch })}
          />
        )}
      </div>
    </SettingsGroup>
  )
}
