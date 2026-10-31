import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  getAvailabilityDefaults,
  setAvailabilityDefaults,
  type AvailabilityDefault,
} from "@/lib/api/availability"

import { toastError } from "@/lib/toast"

import { availabilityKeys } from "./keys"

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
