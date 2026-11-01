import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  clearAvailabilityOverride,
  getAvailabilityDefaults,
  setAvailabilityDefaults,
  setAvailabilityOverride,
  type AvailabilityDefault,
  type AvailabilityMode,
  type AvailabilitySource,
} from "@/lib/api/availability"
import {
  defaultsToGridValue,
  gridValueToDefaults,
} from "@/lib/availability-grid"
import { toastError } from "@/lib/toast"

import { availabilityKeys } from "./keys"
import { invalidatePlanViews } from "./plan"

/** The full weekday × active-slot defaults grid. */
export function useAvailabilityDefaults() {
  return useQuery({
    queryKey: availabilityKeys.defaults(),
    queryFn: getAvailabilityDefaults,
  })
}

const SET_DEFAULTS_KEY = ["availability", "defaults", "set"] as const

/** The whole grid, or an edit of the latest cached grid. */
export type AvailabilityDefaultsUpdate =
  | AvailabilityDefault[]
  | ((current: AvailabilityDefault[]) => AvailabilityDefault[])

type DefaultsData = { items: AvailabilityDefault[] }

/**
 * Replaces the whole defaults grid. An edit is applied to the cache at once
 * and saves run one after another, each sending the full grid as cached when
 * it starts, so quick edits made against the same render all land and the
 * last write wins. Resolved ranges depend on the defaults, so this drops
 * every range query. A failure toasts and refetches the stored grid.
 */
export function useSetAvailabilityDefaults() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const resolve = (update: AvailabilityDefaultsUpdate) => {
    if (typeof update !== "function") return update
    const current = qc.getQueryData<DefaultsData>(availabilityKeys.defaults())
    return update(current?.items ?? [])
  }
  return useMutation({
    mutationKey: SET_DEFAULTS_KEY,
    scope: { id: "availability-defaults" },
    mutationFn: (update: AvailabilityDefaultsUpdate) =>
      setAvailabilityDefaults(resolve(update)),
    // Runs synchronously in mutate(), before the queue wait, so the grid
    // re-renders with the edit in the same event.
    onMutate: async (update) => {
      const cancelled = qc.cancelQueries({
        queryKey: availabilityKeys.defaults(),
      })
      qc.setQueryData<DefaultsData>(availabilityKeys.defaults(), {
        items: resolve(update),
      })
      await cancelled
    },
    onSuccess: (data) => {
      // A queued save is already in the cache; this older response would
      // flash the grid back until it lands.
      if (qc.isMutating({ mutationKey: SET_DEFAULTS_KEY }) <= 1) {
        qc.setQueryData(availabilityKeys.defaults(), data)
      }
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => {
      toastError(err, t)
      void qc.invalidateQueries({ queryKey: availabilityKeys.defaults() })
    },
  })
}

/** One window cell the plan wizard changed (D31). */
export interface WindowCellChange {
  date: string
  slotId: number
  mode: AvailabilityMode
  presetId: number | null
  /** Where the cell's current mode comes from. */
  source: AvailabilitySource
  /** The cell holds a plate, which a No plate override must remove. */
  hasPlate: boolean
}

interface ApplyWindowAvailabilityInput {
  changes: WindowCellChange[]
  /** "Make this my new default": write the weekday defaults instead of
   *  date overrides. */
  asDefault: boolean
  /** The active slots, for the full defaults grid. */
  slotIds: number[]
}

/** 0 = Monday … 6 = Sunday, as the defaults count weekdays. */
function weekdayOf(date: string) {
  const [y, m, d] = date.split("-").map(Number)
  return (new Date(Date.UTC(y, m - 1, d)).getUTCDay() + 6) % 7
}

/**
 * Plan wizard step 1 (D31): writes the changed cells as date overrides, or
 * with `asDefault` as the weekday defaults (an override on a changed date
 * is cleared then, so the new default shows on it). Writes run one after
 * another; the plan views refetch afterwards either way.
 */
export function useApplyWindowAvailability() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: async ({
      changes,
      asDefault,
      slotIds,
    }: ApplyWindowAvailabilityInput) => {
      if (asDefault) {
        const { items } = await getAvailabilityDefaults()
        const patch = Object.fromEntries(
          changes.map((c) => [
            `${weekdayOf(c.date)}:${c.slotId}`,
            c.mode === "routine" && c.presetId !== null
              ? { mode: c.mode, presetId: c.presetId }
              : { mode: c.mode },
          ])
        )
        await setAvailabilityDefaults(
          gridValueToDefaults(
            { ...defaultsToGridValue(items), ...patch },
            slotIds.map((id) => ({ id }))
          )
        )
        for (const c of changes) {
          if (c.source === "override") {
            await clearAvailabilityOverride(c.date, c.slotId)
          }
        }
        return
      }
      for (const c of changes) {
        await setAvailabilityOverride(c.date, c.slotId, {
          mode: c.mode,
          preset_id: c.mode === "routine" ? c.presetId : null,
          remove_plate: c.mode === "none" && c.hasPlate,
        })
      }
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: availabilityKeys.all })
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}
