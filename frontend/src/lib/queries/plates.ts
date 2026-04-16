import { useMutation, useQueryClient } from "@tanstack/react-query"

import {
  addPlateComponent,
  deletePlate,
  deletePlateComponent,
  updatePlate,
  updatePlateComponent,
  type AddPlateComponentInput,
  type UpdatePlateComponentInput,
  type UpdatePlateInput,
} from "@/lib/api/plates"
import type { Week } from "@/lib/api/weeks"

import { weekKeys } from "./keys"
import {
  patchAddComponent,
  patchDeletePlate,
  patchRemoveComponent,
  patchSwapComponent,
  patchUpdateComponentPortions,
  patchUpdatePlate,
} from "./plate-patches"

// Snapshot/rollback helpers shared by every optimistic plate mutation.
// They cancel in-flight queries, snapshot the current cache, apply a patch
// optimistically, and on error roll the snapshot back. On settle they
// invalidate every cache slot related to the touched week.

type WeekSnapshot = { byId?: Week; current?: Week }

function useWeekMutationContext(weekId: number) {
  const qc = useQueryClient()

  return {
    qc,
    snapshot: async (
      patch: (w: Week | undefined) => Week | undefined
    ): Promise<WeekSnapshot> => {
      await qc.cancelQueries({ queryKey: weekKeys.byId(weekId) })
      await qc.cancelQueries({ queryKey: weekKeys.current() })
      const previous: WeekSnapshot = {
        byId: qc.getQueryData<Week>(weekKeys.byId(weekId)),
        current: qc.getQueryData<Week>(weekKeys.current()),
      }
      qc.setQueryData<Week | undefined>(weekKeys.byId(weekId), (old) =>
        patch(old)
      )
      // Only patch "current" if it points at the same week id.
      qc.setQueryData<Week | undefined>(weekKeys.current(), (old) =>
        old?.id === weekId ? patch(old) : old
      )
      return previous
    },
    rollback: (previous: WeekSnapshot) => {
      qc.setQueryData(weekKeys.byId(weekId), previous.byId)
      qc.setQueryData(weekKeys.current(), previous.current)
    },
    invalidate: () => {
      // Single page can read the week via byId, byDate, or current — invalidate
      // all week-rooted queries so whichever cache slot is active refetches.
      void qc.invalidateQueries({ queryKey: weekKeys.all })
    },
  }
}

export function useUpdatePlate(weekId: number) {
  const ctx = useWeekMutationContext(weekId)
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdatePlateInput }) =>
      updatePlate(id, input),
    onMutate: async ({ id, input }) =>
      ctx.snapshot((w) => patchUpdatePlate(w, id, input)),
    onError: (_err, _vars, previous) => previous && ctx.rollback(previous),
    onSettled: ctx.invalidate,
  })
}

export function useDeletePlate(weekId: number) {
  const ctx = useWeekMutationContext(weekId)
  return useMutation({
    mutationFn: (id: number) => deletePlate(id),
    onMutate: async (id) => ctx.snapshot((w) => patchDeletePlate(w, id)),
    onError: (_err, _vars, previous) => previous && ctx.rollback(previous),
    onSettled: ctx.invalidate,
  })
}

export function useAddPlateComponent(weekId: number) {
  const ctx = useWeekMutationContext(weekId)
  return useMutation({
    mutationFn: ({
      plateId,
      input,
    }: {
      plateId: number
      input: AddPlateComponentInput
    }) => addPlateComponent(plateId, input),
    onMutate: async ({ plateId, input }) =>
      ctx.snapshot((w) =>
        patchAddComponent(w, plateId, {
          id: -Date.now(), // temporary negative id distinguishes optimistic rows
          plate_id: plateId,
          component_id: input.component_id,
          portions: input.portions,
          sort_order: 9999,
        })
      ),
    onError: (_err, _vars, previous) => previous && ctx.rollback(previous),
    onSettled: ctx.invalidate,
  })
}

export function useSwapPlateComponent(weekId: number) {
  const ctx = useWeekMutationContext(weekId)
  return useMutation({
    mutationFn: ({
      plateId,
      pcId,
      input,
    }: {
      plateId: number
      pcId: number
      input: UpdatePlateComponentInput
    }) => updatePlateComponent(plateId, pcId, input),
    onMutate: async ({ pcId, input }) =>
      ctx.snapshot((w) =>
        input.component_id !== undefined
          ? patchSwapComponent(w, pcId, input.component_id, input.portions)
          : input.portions !== undefined
            ? patchUpdateComponentPortions(w, pcId, input.portions)
            : w
      ),
    onError: (_err, _vars, previous) => previous && ctx.rollback(previous),
    onSettled: ctx.invalidate,
  })
}

export function useUpdatePlateComponentPortions(weekId: number) {
  const ctx = useWeekMutationContext(weekId)
  return useMutation({
    mutationFn: ({
      plateId,
      pcId,
      portions,
    }: {
      plateId: number
      pcId: number
      portions: number
    }) => updatePlateComponent(plateId, pcId, { portions }),
    onMutate: async ({ pcId, portions }) =>
      ctx.snapshot((w) => patchUpdateComponentPortions(w, pcId, portions)),
    onError: (_err, _vars, previous) => previous && ctx.rollback(previous),
    onSettled: ctx.invalidate,
  })
}

export function useRemovePlateComponent(weekId: number) {
  const ctx = useWeekMutationContext(weekId)
  return useMutation({
    mutationFn: ({ plateId, pcId }: { plateId: number; pcId: number }) =>
      deletePlateComponent(plateId, pcId),
    onMutate: async ({ pcId }) =>
      ctx.snapshot((w) => patchRemoveComponent(w, pcId)),
    onError: (_err, _vars, previous) => previous && ctx.rollback(previous),
    onSettled: ctx.invalidate,
  })
}
