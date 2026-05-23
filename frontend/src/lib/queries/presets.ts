import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

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
  undoApply,
  updatePreset,
  type ApplyPresetInput,
  type ApplySnapshot,
  type CopyWeekInput,
  type CreatePresetInput,
  type ListPresetsParams,
  type PatchPresetInput,
  type UpdatePresetInput,
} from "@/lib/api/presets"

import { plateKeys, presetKeys } from "./keys"

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
  return useMutation({
    mutationFn: (input: CreatePresetInput) => createPreset(input),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
      void qc.invalidateQueries({ queryKey: presetKeys.knownTags() })
    },
  })
}

export function useUpdatePreset() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdatePresetInput }) =>
      updatePreset(id, input),
    onSettled: (_data, _err, vars) => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
      void qc.invalidateQueries({ queryKey: presetKeys.detail(vars.id) })
      void qc.invalidateQueries({ queryKey: presetKeys.knownTags() })
    },
  })
}

export function usePatchPreset() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: PatchPresetInput }) =>
      patchPreset(id, input),
    onSettled: (_data, _err, vars) => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
      void qc.invalidateQueries({ queryKey: presetKeys.detail(vars.id) })
      void qc.invalidateQueries({ queryKey: presetKeys.knownTags() })
    },
  })
}

export function useDeletePreset() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deletePreset(id),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
    },
  })
}

export function useDuplicatePreset() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => duplicatePreset(id),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: presetKeys.lists() })
    },
  })
}

/** Apply a preset to a target date. Invalidates plates + nutrition so the
 * planner refetches after the mutation. */
export function useApplyPreset() {
  const qc = useQueryClient()
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
  })
}

export function useCopyWeek() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CopyWeekInput) => copyWeek(input),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}

export function useUndoApply() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (snapshot: ApplySnapshot) => undoApply(snapshot),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}
