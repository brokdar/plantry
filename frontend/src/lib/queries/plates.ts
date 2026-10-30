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
  getAvailabilityRange,
  setAvailabilityOverride,
  type AvailabilityCell,
  type AvailabilityRangeResponse,
} from "@/lib/api/availability"
import { restorePlan } from "@/lib/api/plan"
import {
  addPlateComponent,
  createPlate,
  deletePlate,
  deletePlateComponent,
  listPlateMacros,
  listPlates,
  updatePlate,
  updatePlateComponent,
  type ComponentInput,
  type CreatePlateInput,
  type PlanSnapshot,
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
      invalidatePlanViews(qc)
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

// ── Old planner (kept until F4 deletes it) ───────────────────────────
// These also invalidate the plan views so Today, Plan and Shop stay current.

export function usePlannerUpdatePlate(rangeFrom?: string, rangeTo?: string) {
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
      invalidatePlanViews(qc)
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
      invalidatePlanViews(qc)
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
      invalidatePlanViews(qc)
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
      invalidatePlanViews(qc)
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
 *   with its mode, preset and note), then the removed plate is restored from
 *   the snapshot the server returned (`POST /api/plan/restore`), sessions included.
 * - `planMeal` drops a cell's override so it falls back to its weekday
 *   default, and puts a `plan` override only when that default is `none`.
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
    removed: PlanSnapshot | null
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
      if (removed) await restorePlan(removed)
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
    let removed: PlanSnapshot | null = null
    try {
      const res = await setOverride.mutateAsync({
        date,
        slotId,
        input: { mode: "none", note: note ?? null, remove_plate: true },
      })
      removed = res.removed
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
    // Prefer reverting to the weekday default so the date is not pinned;
    // only a `none` default needs an explicit `plan` override.
    try {
      if (cellAt(date, slotId)?.source === "override") {
        await clearOverride.mutateAsync({ date, slotId })
        let resolved: AvailabilityCell | undefined
        try {
          const res = await getAvailabilityRange(date, date)
          resolved = res.items.find((c) => c.slot_id === slotId)
        } catch (err) {
          toastError(err, t)
          return
        }
        if (resolved?.mode !== "none") return
      }
      await setOverride.mutateAsync({ date, slotId, input: { mode: "plan" } })
    } catch {
      // toasted by the mutation hooks
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
