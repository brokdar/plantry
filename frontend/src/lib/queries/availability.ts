import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  clearAvailabilityOverride,
  getAvailabilityDefaults,
  getAvailabilityRange,
  setAvailabilityDefaults,
  setAvailabilityOverride,
  type AvailabilityDefault,
  type SetAvailabilityOverrideInput,
} from "@/lib/api/availability"

import { toastError } from "@/lib/toast"

import { availabilityKeys, plateKeys } from "./keys"

/** The full weekday × active-slot defaults grid. */
export function useAvailabilityDefaults() {
  return useQuery({
    queryKey: availabilityKeys.defaults(),
    queryFn: getAvailabilityDefaults,
  })
}

/** Replaces the whole defaults grid. Resolved ranges depend on the
 *  defaults, so this drops every availability query. */
export function useSetAvailabilityDefaults() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (items: AvailabilityDefault[]) =>
      setAvailabilityDefaults(items),
    onSuccess: (data) => {
      qc.setQueryData(availabilityKeys.defaults(), data)
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => toastError(err, t),
  })
}

/** Resolved cells for every date × active slot in [from, to] (≤ 62 days). */
export function useAvailabilityRange(from: string, to: string) {
  return useQuery({
    queryKey: availabilityKeys.range(from, to),
    queryFn: () => getAvailabilityRange(from, to),
    enabled: !!from && !!to,
  })
}

export interface SetAvailabilityOverrideVars {
  date: string
  slotId: number
  input: SetAvailabilityOverrideInput
}

/** Sets one date × slot override. When the server removed the cell's plate
 *  (`remove_plate: true`), plate queries are dropped too. */
export function useSetAvailabilityOverride() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ date, slotId, input }: SetAvailabilityOverrideVars) =>
      setAvailabilityOverride(date, slotId, input),
    onSuccess: (data) => {
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
      if (data.removed_plate) {
        void qc.invalidateQueries({ queryKey: plateKeys.all })
      }
    },
    onError: (err) => toastError(err, t),
  })
}

/** Removes one override so the cell falls back to its weekday default. */
export function useClearAvailabilityOverride() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ date, slotId }: { date: string; slotId: number }) =>
      clearAvailabilityOverride(date, slotId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => toastError(err, t),
  })
}
