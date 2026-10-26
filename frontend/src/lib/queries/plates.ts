import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  clearAvailabilityOverride,
  setAvailabilityOverride,
  type AvailabilityCell,
  type AvailabilityRangeResponse,
} from "@/lib/api/availability"
import {
  addPlateComponent,
  componentToAddInput,
  createPlate,
  deletePlate,
  deletePlateComponent,
  listPlateMacros,
  listPlates,
  updatePlate,
  updatePlateComponent,
  type AddPlateComponentInput,
  type Plate,
  type PlateComponentQuantity,
  type UpdatePlateComponentInput,
  type UpdatePlateInput,
} from "@/lib/api/plates"

import { toast, toastError } from "@/lib/toast"

import {
  useClearAvailabilityOverride,
  useSetAvailabilityOverride,
} from "./availability"
import { availabilityKeys, plateKeys } from "./keys"
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
 *  (add/remove/update component, swap, move) also drops these macros. */
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
  const { t } = useTranslation()
  return useMutation({
    mutationFn: createPlate,
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({
        queryKey: plateKeys.range(rangeFrom, rangeTo),
      })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useUpdatePlate(rangeFrom?: string, rangeTo?: string) {
  const qc = useQueryClient()
  const { t } = useTranslation()
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
    onError: (err) => toastError(err, t),
  })
}

export function useDeletePlate() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => deletePlate(id),
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useAddPlateComponent() {
  const qc = useQueryClient()
  const { t } = useTranslation()
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
    onError: (err) => toastError(err, t),
  })
}

export function useSwapPlateComponent() {
  const qc = useQueryClient()
  const { t } = useTranslation()
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
    onError: (err) => toastError(err, t),
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
  const { t } = useTranslation()
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
    onError: (err) => toastError(err, t),
  })
}

export function useRemovePlateComponent() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ plateId, pcId }: { plateId: number; pcId: number }) =>
      deletePlateComponent(plateId, pcId),
    onMutate: () => flushPendingPlateDeletes(),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    },
    onError: (err) => toastError(err, t),
  })
}

/** How long the "Mark as No plate" undo stays on offer (UX §6). */
const NO_PLATE_UNDO_MS = 6000

/**
 * The planner's No-plate actions for one window (D9). The cell's current
 * availability is read from the window's availability range cache, which the
 * planner route has already loaded.
 *
 * - `markNoPlate` puts a `none` override with `remove_plate: true` and offers
 *   Undo: first the previous override state comes back (cleared, or re-put
 *   with its mode, preset and note), then the removed plate is recreated with
 *   its components. Its feedback is not restored.
 * - `planMeal` drops the cell's override, or puts `plan` over a `none` default.
 * - `editNote` keeps the cell `none` and writes its note.
 *
 * Failures are toasted by the underlying mutation hooks.
 */
export function useNoPlateActions(rangeFrom: string, rangeTo: string) {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const setOverride = useSetAvailabilityOverride()
  const clearOverride = useClearAvailabilityOverride()

  function cellAt(date: string, slotId: number): AvailabilityCell | undefined {
    return qc
      .getQueryData<AvailabilityRangeResponse>(
        availabilityKeys.range(rangeFrom, rangeTo)
      )
      ?.items.find((c) => c.date === date && c.slot_id === slotId)
  }

  async function undoMarkNoPlate(
    date: string,
    slotId: number,
    previous: AvailabilityCell | undefined,
    removed: Plate | undefined
  ) {
    try {
      if (previous?.source === "override") {
        await setAvailabilityOverride(date, slotId, {
          mode: previous.mode,
          preset_id: previous.preset_id,
          note: previous.note,
        })
      } else {
        await clearAvailabilityOverride(date, slotId)
      }
      if (removed) {
        const created = await createPlate({
          date,
          slot_id: slotId,
          note: removed.note ?? undefined,
        })
        const components = [...removed.components].sort(
          (a, b) => a.sort_order - b.sort_order
        )
        for (const pc of components) {
          await addPlateComponent(created.id, componentToAddInput(pc))
        }
      }
    } catch (err) {
      toastError(err, t)
    } finally {
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
      void qc.invalidateQueries({ queryKey: plateKeys.all })
      void qc.invalidateQueries({ queryKey: ["nutrition"] })
    }
  }

  async function markNoPlate(
    date: string,
    slotId: number,
    note?: string | null
  ) {
    const previous = cellAt(date, slotId)
    await flushPendingPlateDeletes()
    let removed: Plate | undefined
    try {
      const res = await setOverride.mutateAsync({
        date,
        slotId,
        input: { mode: "none", note: note ?? null, remove_plate: true },
      })
      removed = res.removed_plate
    } catch {
      return
    }
    void qc.invalidateQueries({ queryKey: ["nutrition"] })
    toast(t("planner.availability.marked"), {
      action: {
        label: t("common.undo"),
        onClick: () => void undoMarkNoPlate(date, slotId, previous, removed),
      },
      duration: NO_PLATE_UNDO_MS,
    })
  }

  async function planMeal(date: string, slotId: number) {
    try {
      if (cellAt(date, slotId)?.source === "override") {
        await clearOverride.mutateAsync({ date, slotId })
      } else {
        await setOverride.mutateAsync({ date, slotId, input: { mode: "plan" } })
      }
    } catch {
      // toasted by the mutation hook
    }
  }

  async function editNote(date: string, slotId: number, note: string | null) {
    try {
      await setOverride.mutateAsync({
        date,
        slotId,
        input: { mode: "none", note },
      })
    } catch {
      // toasted by the mutation hook
    }
  }

  return { markNoPlate, planMeal, editNote }
}
