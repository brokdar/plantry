import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  clearAvailabilityOverride,
  getAvailabilityDefaults,
  getAvailabilityRange,
  setAvailabilityDefaults,
  setAvailabilityOverride,
  type AvailabilityCell,
  type AvailabilityDefault,
  type AvailabilityMode,
} from "@/lib/api/availability"
import { restorePlan, type PlanCell } from "@/lib/api/plan"
import type { PlanSnapshot } from "@/lib/api/plates"
import {
  defaultsToGridValue,
  gridValueToDefaults,
} from "@/lib/availability-grid"
import { toast, toastError } from "@/lib/toast"

import { availabilityKeys } from "./keys"
import { invalidatePlanViews, toastUndoError, useUndoToast } from "./plan"

/** The full weekday × active-slot defaults grid. */
export function useAvailabilityDefaults() {
  return useQuery({
    queryKey: availabilityKeys.defaults(),
    queryFn: getAvailabilityDefaults,
  })
}

const SET_DEFAULTS_KEY = ["availability", "defaults", "set"] as const

/** The whole grid, or an edit of the latest cached grid. */
export type AvailabilityDefaultsUpdate =
  | AvailabilityDefault[]
  | ((current: AvailabilityDefault[]) => AvailabilityDefault[])

type DefaultsData = { items: AvailabilityDefault[] }

/**
 * Replaces the whole defaults grid. An edit is applied to the cache at once
 * and saves run one after another, each sending the full grid as cached when
 * it starts, so quick edits made against the same render all land and the
 * last write wins. Resolved ranges depend on the defaults, so this drops
 * every range query. A failure toasts and refetches the stored grid.
 */
export function useSetAvailabilityDefaults() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const resolve = (update: AvailabilityDefaultsUpdate) => {
    if (typeof update !== "function") return update
    const current = qc.getQueryData<DefaultsData>(availabilityKeys.defaults())
    return update(current?.items ?? [])
  }
  return useMutation({
    mutationKey: SET_DEFAULTS_KEY,
    scope: { id: "availability-defaults" },
    mutationFn: (update: AvailabilityDefaultsUpdate) =>
      setAvailabilityDefaults(resolve(update)),
    // Runs synchronously in mutate(), before the queue wait, so the grid
    // re-renders with the edit in the same event.
    onMutate: async (update) => {
      const cancelled = qc.cancelQueries({
        queryKey: availabilityKeys.defaults(),
      })
      qc.setQueryData<DefaultsData>(availabilityKeys.defaults(), {
        items: resolve(update),
      })
      await cancelled
    },
    onSuccess: (data) => {
      // A queued save is already in the cache; this older response would
      // flash the grid back until it lands.
      if (qc.isMutating({ mutationKey: SET_DEFAULTS_KEY }) <= 1) {
        qc.setQueryData(availabilityKeys.defaults(), data)
      }
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => {
      toastError(err, t)
      void qc.invalidateQueries({ queryKey: availabilityKeys.defaults() })
    },
  })
}

/** One window cell the plan wizard changed (D31). */
export interface WindowCellChange {
  date: string
  slotId: number
  mode: AvailabilityMode
  presetId: number | null
  /** The cell as the server resolves it now (source, override, plate). */
  cell: PlanCell
}

interface ApplyWindowAvailabilityInput {
  changes: WindowCellChange[]
  /** "Make this my new default": write the weekday defaults instead of
   *  date overrides. */
  asDefault: boolean
  /** The active slots, for the full defaults grid. */
  slotIds: number[]
  /** Defaults an earlier pass replaced, put back first (the user switched
   *  from defaults back to overrides). */
  revertDefaults?: AvailabilityDefault[]
}

interface ApplyWindowAvailabilityResult {
  removed: RemovedCell[]
  /** The defaults before this pass wrote them; null for overrides. */
  previousDefaults: AvailabilityDefault[] | null
}

/** A plate a No plate override removed, with the cell as it was. */
interface RemovedCell {
  cell: PlanCell
  snapshot: PlanSnapshot
}

/** 0 = Monday … 6 = Sunday, as the defaults count weekdays. */
function weekdayOf(date: string) {
  const [y, m, d] = date.split("-").map(Number)
  return (new Date(Date.UTC(y, m - 1, d)).getUTCDay() + 6) % 7
}

const cellKey = (date: string, slotId: number) => `${date}:${slotId}`

/** Writes the changes into the defaults; returns the defaults before. */
async function writeDefaults(
  changes: WindowCellChange[],
  slotIds: number[]
): Promise<AvailabilityDefault[]> {
  const { items } = await getAvailabilityDefaults()
  const patch = Object.fromEntries(
    changes.map((c) => [
      `${weekdayOf(c.date)}:${c.slotId}`,
      c.mode === "routine" && c.presetId !== null
        ? { mode: c.mode, presetId: c.presetId }
        : { mode: c.mode },
    ])
  )
  await setAvailabilityDefaults(
    gridValueToDefaults(
      { ...defaultsToGridValue(items), ...patch },
      slotIds.map((id) => ({ id }))
    )
  )
  // A date override would hide the new default on that date.
  for (const c of changes) {
    if (c.cell.source === "override") {
      await clearAvailabilityOverride(c.date, c.slotId)
    }
  }
  return items
}

/** Plates linked through a cook session restore only together: each
 *  snapshot names the session, which removing the next plate deletes. */
function sharesBatch(removed: RemovedCell[]) {
  const seen = new Set<number>()
  for (const { cell } of removed) {
    const ids = new Set(
      (cell.plate?.components ?? []).flatMap((c) =>
        c.cook_session_id === undefined ? [] : [c.cook_session_id]
      )
    )
    for (const id of ids) {
      if (seen.has(id)) return true
      seen.add(id)
    }
  }
  return false
}

/**
 * Date overrides for the changed cells. A cell that had an override is
 * first given back to its weekday default (as "Plan this meal" does) and
 * pinned again only when the default differs. A No plate cell that holds
 * a plate is always written with `remove_plate`; the removed plates are
 * returned for Undo.
 */
async function writeOverrides(
  changes: WindowCellChange[]
): Promise<RemovedCell[]> {
  const removes = (c: WindowCellChange) =>
    c.mode === "none" && c.cell.plate !== null
  const cleared = changes.filter(
    (c) => c.cell.source === "override" && !removes(c)
  )
  for (const c of cleared) await clearAvailabilityOverride(c.date, c.slotId)
  const resolved = new Map<string, AvailabilityCell>()
  if (cleared.length > 0) {
    const dates = cleared.map((c) => c.date).sort()
    const { items } = await getAvailabilityRange(
      dates[0],
      dates[dates.length - 1]
    )
    for (const item of items) {
      resolved.set(cellKey(item.date, item.slot_id), item)
    }
  }

  const removed: RemovedCell[] = []
  for (const c of changes) {
    const now = resolved.get(cellKey(c.date, c.slotId))
    if (
      now &&
      now.mode === c.mode &&
      (c.mode !== "routine" || now.preset_id === c.presetId)
    ) {
      continue
    }
    const res = await setAvailabilityOverride(c.date, c.slotId, {
      mode: c.mode,
      preset_id: c.mode === "routine" ? c.presetId : null,
      remove_plate: removes(c),
    })
    if (res.removed) removed.push({ cell: c.cell, snapshot: res.removed })
  }
  return removed
}

/**
 * Plan wizard step 1 (D31): writes the changed cells as date overrides, or
 * with `asDefault` as the weekday defaults (plates stay then). Writes run
 * one after another; the plan views refetch afterwards either way. Plates a
 * No plate override removed get one Undo toast: it puts each cell's
 * previous availability back and restores its plate.
 */
export function useApplyWindowAvailability() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offer = useUndoToast()

  function refresh() {
    void qc.invalidateQueries({ queryKey: availabilityKeys.all })
    return invalidatePlanViews(qc)
  }

  /** Per cell: its previous availability back, then its plate. The
   *  availability goes first because a No plate cell refuses the plate; a
   *  failed restore marks that cell No plate again, so it never ends up
   *  half undone. */
  async function undo(removed: RemovedCell[]) {
    try {
      for (const { cell, snapshot } of removed) {
        if (cell.source === "override") {
          await setAvailabilityOverride(cell.date, cell.slot_id, {
            mode: cell.mode,
            preset_id: cell.preset_id,
            note: cell.note,
          })
        } else {
          await clearAvailabilityOverride(cell.date, cell.slot_id)
        }
        try {
          await restorePlan(snapshot)
        } catch (err) {
          await setAvailabilityOverride(cell.date, cell.slot_id, {
            mode: "none",
            note: cell.note,
          }).catch(() => {})
          throw err
        }
      }
      toast.success(t("plan.undo.restored"))
    } catch (err) {
      toastUndoError(err, t)
    } finally {
      void refresh()
    }
  }

  return useMutation({
    mutationFn: async ({
      changes,
      asDefault,
      slotIds,
      revertDefaults,
    }: ApplyWindowAvailabilityInput): Promise<ApplyWindowAvailabilityResult> => {
      if (revertDefaults) await setAvailabilityDefaults(revertDefaults)
      if (!asDefault) {
        return {
          removed: await writeOverrides(changes),
          previousDefaults: null,
        }
      }
      return {
        removed: [],
        previousDefaults: await writeDefaults(changes, slotIds),
      }
    },
    onSuccess: ({ removed }) => {
      if (removed.length === 0) return
      const message = t("plan.wizard.removed", { count: removed.length })
      // Separate snapshots of one batch can't be restored one by one; a
      // batch Undo would need one multi-cell removal on the server.
      if (sharesBatch(removed)) toast(message)
      else offer(message, () => void undo(removed))
    },
    onSettled: refresh,
    onError: (err) => toastError(err, t),
  })
}
