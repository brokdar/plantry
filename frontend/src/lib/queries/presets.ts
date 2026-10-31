import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  applyPreset,
  copyWeek,
  createPreset,
  deletePreset,
  duplicatePreset,
  getKnownTags,
  getPreset,
  getPresets,
  patchPreset,
  updatePreset,
  type ApplyPresetInput,
  type ApplyResult,
  type CopyWeekInput,
  type CreatePresetInput,
  type ListPresetsParams,
  type PatchPresetInput,
  type UpdatePresetInput,
} from "@/lib/api/presets"

import { clearAvailabilityOverride } from "@/lib/api/availability"
import { restorePlan } from "@/lib/api/plan"
import { deletePlate } from "@/lib/api/plates"
import { toast, toastError } from "@/lib/toast"

import { availabilityKeys, presetKeys } from "./keys"
import { invalidatePlanViews, useUndoToast } from "./plan"

/** List presets with optional search / slot / tag filters and pagination. */
export function usePresets(params: ListPresetsParams = {}) {
  return useQuery({
    queryKey: presetKeys.list(params),
    queryFn: () => getPresets(params),
  })
}

export function usePreset(id: number) {
  return useQuery({
    queryKey: presetKeys.detail(id),
    queryFn: () => getPreset(id),
    enabled: id > 0,
  })
}

export function useKnownTags() {
  return useQuery({
    queryKey: presetKeys.knownTags(),
    queryFn: () => getKnownTags(),
  })
}

export function useCreatePreset() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (input: CreatePresetInput) => createPreset(input),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
      void qc.invalidateQueries({ queryKey: presetKeys.knownTags() })
    },
    onError: (err) => toastError(err, t),
  })
}

/** Full editor save. Pass the routine's id to run its saves one at a time,
 *  in order, so the last full-replacement PUT always lands last. */
export function useUpdatePreset(serialId?: number) {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    scope: serialId ? { id: `routine-${serialId}` } : undefined,
    mutationFn: ({ id, input }: { id: number; input: UpdatePresetInput }) =>
      updatePreset(id, input),
    onSettled: (_data, _err, vars) => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
      void qc.invalidateQueries({ queryKey: presetKeys.detail(vars.id) })
      void qc.invalidateQueries({ queryKey: presetKeys.knownTags() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function usePatchPreset() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: PatchPresetInput }) =>
      patchPreset(id, input),
    onSettled: (_data, _err, vars) => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
      void qc.invalidateQueries({ queryKey: presetKeys.detail(vars.id) })
      void qc.invalidateQueries({ queryKey: presetKeys.knownTags() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useDeletePreset() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => deletePreset(id),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useDuplicatePreset() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => duplicatePreset(id),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
    },
    onError: (err) => toastError(err, t),
  })
}

/** Apply a preset to a target date; refreshes the plan views. */
export function useApplyPreset() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({
      presetId,
      input,
    }: {
      presetId: number
      input: ApplyPresetInput
    }) => applyPreset(presetId, input),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
      void invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

export function useCopyWeek() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (input: CopyWeekInput) => copyWeek(input),
    onSettled: () => {
      void invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

/** Undoes an apply or copy week: deletes the created plates, restores what
 *  `overwrite` replaced and clears the No plate overrides copy week wrote. */
export function useUndoApply() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: async (result: ApplyResult) => {
      for (const id of result.created_plate_ids) await deletePlate(id)
      if (result.removed.plates.length > 0) await restorePlan(result.removed)
      for (const c of result.created_overrides) {
        await clearAvailabilityOverride(c.date, c.slot_id)
      }
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
      void invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

/**
 * The one toast flow for an apply or copy week result (plate sheet, command
 * palette, copy week dialog). It names what was added and replaced, offers
 * Undo (`useUndoApply`) through the shared undo toast whenever plates were
 * created, then warns about the cells it skipped.
 */
export function useApplyResultToasts() {
  const { t } = useTranslation()
  const offerUndo = useUndoToast()
  const undo = useUndoApply()

  return function show(result: ApplyResult) {
    const replaced = result.removed.plates.length
    const added = Math.max(0, result.created_plate_ids.length - replaced)
    if (result.created_plate_ids.length > 0) {
      const message = [
        added > 0 && t("routines.apply.result_created", { count: added }),
        replaced > 0 &&
          t("routines.apply.result_replaced", { count: replaced }),
      ]
        .filter(Boolean)
        .join(" ")
      offerUndo(message, () => undo.mutate(result))
    }
    const skipped = [
      ["result_skipped_occupied", result.skipped_occupied.length],
      ["result_skipped_unavailable", result.skipped_unavailable.length],
      ["result_skipped_no_slot", result.skipped_no_slot.length],
    ] as const
    for (const [key, count] of skipped) {
      if (count > 0) toast.warning(t(`routines.apply.${key}`, { count }))
    }
  }
}
