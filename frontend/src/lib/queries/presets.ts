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
import { toastError } from "@/lib/toast"

import { availabilityKeys, plateKeys, presetKeys } from "./keys"
import { invalidatePlanViews } from "./plan"

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

export function useUpdatePreset() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
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

/** Apply a preset to a target date. Invalidates plates + nutrition so the
 * planner refetches after the mutation. */
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
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
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
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
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
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
      invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}
