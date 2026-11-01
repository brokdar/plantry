import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  addPlateComponent,
  createPlate,
  deletePlate,
  deletePlateComponent,
  updatePlate,
  updatePlateComponent,
  type ComponentInput,
  type CreatePlateInput,
  type UpdatePlateComponentInput,
  type UpdatePlateInput,
} from "@/lib/api/plates"
import { restorePlan } from "@/lib/api/plan"
import { toastError } from "@/lib/toast"

import { invalidatePlanViews, useUndoableRemoval } from "./plan"

// ── Plate mutations for the new screens (§3.1) ───────────────────────
// Each invalidates exactly the plan views (planKeys, todayKeys, shoppingKeys).

export function useCreatePlate() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (input: CreatePlateInput) => createPlate(input),
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => toastError(err, t),
  })
}

/** Add to plan over a draft plate: deletes the draft, then creates the new
 *  plate in its cell. If the create fails, the draft is restored, so the
 *  cell is never left empty by a failed replace. */
export function useReplaceDraftPlate() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: async ({
      draftId,
      input,
    }: {
      draftId: number
      input: CreatePlateInput
    }) => {
      const { removed } = await deletePlate(draftId)
      try {
        return await createPlate(input)
      } catch (err) {
        await restorePlan(removed)
        throw err
      }
    },
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => {
      void invalidatePlanViews(qc)
      toastError(err, t)
    },
  })
}

/** Move (date, slot), eaters and note. */
export function useUpdatePlate() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdatePlateInput }) =>
      updatePlate(id, input),
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => toastError(err, t),
  })
}

/** Deletes the plate and offers Undo through the returned snapshot. */
export function useDeletePlate() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerUndo = useUndoableRemoval()
  return useMutation({
    mutationFn: (id: number) => deletePlate(id),
    onSuccess: (res) => {
      void invalidatePlanViews(qc)
      offerUndo(res, "plate_removed")
    },
    onError: (err) => toastError(err, t),
  })
}

export function useAddComponent() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({
      plateId,
      input,
    }: {
      plateId: number
      input: ComponentInput
    }) => addPlateComponent(plateId, input),
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => toastError(err, t),
  })
}

/** Swap (`food_id`) or quantity (`portions` | `amount` + `unit`). */
export function useUpdateComponent() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({
      plateId,
      componentId,
      input,
    }: {
      plateId: number
      componentId: number
      input: UpdatePlateComponentInput
    }) => updatePlateComponent(plateId, componentId, input),
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => toastError(err, t),
  })
}

export function useRemoveComponent() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({
      plateId,
      componentId,
    }: {
      plateId: number
      componentId: number
    }) => deletePlateComponent(plateId, componentId),
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => toastError(err, t),
  })
}
