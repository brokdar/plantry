import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query"

import {
  addPlateComponent,
  createPlate,
  deletePlate,
  deletePlateComponent,
  listPlateMacros,
  listPlates,
  setPlateSkipped,
  updatePlate,
  updatePlateComponent,
  type AddPlateComponentInput,
  type Plate,
  type PlateComponentQuantity,
  type SetPlateSkippedInput,
  type UpdatePlateComponentInput,
  type UpdatePlateInput,
} from "@/lib/api/plates"

import { plateKeys } from "./keys"
import { flushPendingPlateDeletes } from "./pending-plate-deletes"

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

/** Per-plate macro totals for a date range. The query key sits under
 *  `plateKeys.all`, so any mutation that already invalidates the plates list
 *  (add/remove/update component, swap, move, skip) also drops these macros. */
export function usePlateMacros(from: string, to: string) {
  return useQuery({
    queryKey: plateKeys.macrosRange(from, to),
    queryFn: () => listPlateMacros(from, to),
    enabled: !!from && !!to,
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

export function useCreatePlate(rangeFrom: string, rangeTo: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createPlate,
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({
        queryKey: plateKeys.range(rangeFrom, rangeTo),
      })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}

export function useUpdatePlate(rangeFrom?: string, rangeTo?: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdatePlateInput }) =>
      updatePlate(id, input),
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      if (rangeFrom && rangeTo) {
        void qc.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
      } else {
        void qc.invalidateQueries({ queryKey: plateKeys.all })
      }
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}

export function useDeletePlate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deletePlate(id),
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}

export function useAddPlateComponent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      plateId,
      input,
    }: {
      plateId: number
      input: AddPlateComponentInput
    }) => addPlateComponent(plateId, input),
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}

export function useSwapPlateComponent() {
  const qc = useQueryClient()
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
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}

/**
 * useUpdatePlateComponentQuantity drives the quantity-only patch on a plate
 * component. The shape is kind-aware: composed → `{ portions }`, leaf →
 * `{ amount, unit }`. The backend re-resolves grams server-side for leaf
 * updates so the cache will reflect a fresh `grams`/`grams_source` after
 * invalidation.
 *
 * Replaces the old `useUpdatePlateComponentPortions` which hard-coded the
 * composed-only shape.
 */
export function useUpdatePlateComponentQuantity() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      plateId,
      pcId,
      quantity,
    }: {
      plateId: number
      pcId: number
      quantity: PlateComponentQuantity
    }) => updatePlateComponent(plateId, pcId, quantity),
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}

export function useSetPlateSkipped() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      plateId,
      input,
    }: {
      plateId: number
      input: SetPlateSkippedInput
    }) => setPlateSkipped(plateId, input),
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}

export function useRemovePlateComponent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ plateId, pcId }: { plateId: number; pcId: number }) =>
      deletePlateComponent(plateId, pcId),
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
  })
}
