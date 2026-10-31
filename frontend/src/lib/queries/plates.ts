import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  addPlateComponent,
  createPlate,
  deletePlate,
  deletePlateComponent,
  listPlates,
  updatePlate,
  updatePlateComponent,
  type ComponentInput,
  type CreatePlateInput,
  type Plate,
  type UpdatePlateComponentInput,
  type UpdatePlateInput,
} from "@/lib/api/plates"
import { toastError } from "@/lib/toast"

import { plateKeys } from "./keys"
import { invalidatePlanViews, useUndoableRemoval } from "./plan"

export function usePlatesRange(from: string, to: string) {
  return useQuery({
    queryKey: plateKeys.range(from, to),
    queryFn: () => listPlates(from, to),
    enabled: !!from && !!to,
  })
}

export function usePlatesByDate(date: string) {
  return useQuery({
    queryKey: plateKeys.byDate(date),
    queryFn: () => listPlates(date, date),
    enabled: !!date,
  })
}

interface PlatesPage {
  plates: Plate[]
  from: string
  to: string
}

/** Pages over 60-day chunks backward from `anchor` (YYYY-MM-DD). */
export function usePlatesRangeInfinite(anchor: string) {
  return useInfiniteQuery<
    PlatesPage,
    Error,
    InfiniteData<PlatesPage>,
    ReturnType<typeof plateKeys.rangeInfinite>,
    { from: string; to: string }
  >({
    queryKey: plateKeys.rangeInfinite(anchor),
    queryFn: async ({ pageParam }) => {
      const { from, to } = pageParam
      const data = await listPlates(from, to)
      return { plates: data.plates, from, to }
    },
    initialPageParam: (() => {
      if (!anchor) return { from: "", to: "" }
      const to = anchor
      const d = new Date(anchor)
      d.setDate(d.getDate() - 59)
      const from = d.toISOString().slice(0, 10)
      return { from, to }
    })(),
    getNextPageParam: (lastPage) => {
      if (lastPage.plates.length === 0) return undefined
      const d = new Date(lastPage.from)
      d.setDate(d.getDate() - 1)
      const newTo = d.toISOString().slice(0, 10)
      const d2 = new Date(newTo)
      d2.setDate(d2.getDate() - 59)
      const newFrom = d2.toISOString().slice(0, 10)
      return { from: newFrom, to: newTo }
    },
    enabled: !!anchor,
  })
}

export function flattenPlatesPages(
  data: InfiniteData<PlatesPage> | undefined
): Plate[] {
  if (!data) return []
  return data.pages.flatMap((p) => p.plates)
}

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
