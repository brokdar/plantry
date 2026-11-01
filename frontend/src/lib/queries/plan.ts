import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import type { TFunction } from "i18next"
import { useTranslation } from "react-i18next"

import {
  clearAvailabilityOverride,
  getAvailabilityRange,
  setAvailabilityOverride,
} from "@/lib/api/availability"
import { ApiError } from "@/lib/api/client"
import {
  acceptPlan,
  clearPlan,
  discardPlan,
  getPlan,
  getReplanPreview,
  replan,
  restorePlan,
  unacceptPlan,
  type PlanCell,
  type ReplanCell,
  type ReplanInput,
} from "@/lib/api/plan"
import {
  confirmPlate,
  unlockPlate,
  type PlanSnapshot,
  type RemovedResponse,
  type StateChange,
} from "@/lib/api/plates"
import { confirmSession, unlockSession } from "@/lib/api/sessions"
import { toast, toastError } from "@/lib/toast"

import { availabilityKeys, planKeys, shoppingKeys, todayKeys } from "./keys"

/** Every plan mutation refreshes the read models and the shopping list.
 *  Returned from `onSuccess`, the mutation stays pending until the active
 *  views have refetched, so a control showing a pending value never flashes
 *  back to the old one. */
export function invalidatePlanViews(qc: QueryClient): Promise<unknown> {
  return Promise.all([
    qc.invalidateQueries({ queryKey: planKeys.all }),
    qc.invalidateQueries({ queryKey: todayKeys.all }),
    qc.invalidateQueries({ queryKey: shoppingKeys.all }),
  ])
}

/** The board's read model. Without `start` the server opens the anchored
 *  window. */
export function usePlan(start?: string) {
  return useQuery({
    queryKey: planKeys.window(start),
    queryFn: () => getPlan(start),
  })
}

export function useRestorePlan() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (snapshot: PlanSnapshot) => restorePlan(snapshot),
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => toastError(err, t),
  })
}

/** How long Undo stays on offer after a removal (UX §6). */
export const UNDO_MS = 6000

export type RemovalVerb =
  | "plate_removed"
  | "batch_removed"
  | "plan_cleared"
  | "draft_discarded"

const REMOVAL_MESSAGE_KEYS: Record<RemovalVerb, string> = {
  plate_removed: "plan.undo.plate_removed",
  batch_removed: "plan.undo.batch_removed",
  plan_cleared: "plan.undo.plan_cleared",
  draft_discarded: "plan.undo.draft_discarded",
}

/**
 * The undo toast every undoable plan action shares (UX §6): the returned
 * function shows `message` for UNDO_MS with an Undo action that runs `undo`
 * once, and not after the toast has closed.
 */
export function useUndoToast() {
  const { t } = useTranslation()
  return function offer(message: string, undo: () => void) {
    let closed = false
    const close = () => {
      closed = true
    }
    toast(message, {
      duration: UNDO_MS,
      action: {
        label: t("common.undo"),
        onClick: () => {
          if (closed) return
          closed = true
          undo()
        },
      },
      onDismiss: close,
      onAutoClose: close,
    })
  }
}

/** Restore refuses a cell that was planned or marked No plate since. */
const UNDO_CONFLICT_KEYS = new Set([
  "error.plan.cell_occupied",
  "error.plate.cell_unavailable",
])

/** A failed Undo names the conflict instead of the generic restore error. */
export function toastUndoError(err: unknown, t: TFunction) {
  if (err instanceof ApiError && UNDO_CONFLICT_KEYS.has(err.messageKey)) {
    toast.error(t("plan.undo.conflict"))
    return
  }
  toastError(err, t)
}

/**
 * The single undo helper for removing mutations (D22). The returned function
 * takes the `{removed}` body, shows the undo toast named after what happened
 * and, on Undo, restores the snapshot and confirms it. An empty snapshot
 * offers nothing.
 */
export function useUndoableRemoval() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offer = useUndoToast()
  const restore = useMutation({
    mutationFn: (snapshot: PlanSnapshot) => restorePlan(snapshot),
    onSuccess: () => {
      toast.success(t("plan.undo.restored"))
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastUndoError(err, t),
  })

  return function offerUndo({ removed }: RemovedResponse, verb: RemovalVerb) {
    if (removed.plates.length === 0 && removed.sessions.length === 0) return
    offer(t(REMOVAL_MESSAGE_KEYS[verb]), () => restore.mutate(removed))
  }
}

/** Clear window: removes every plate in [from, to] and offers Undo. */
export function useClearWindow() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerUndo = useUndoableRemoval()
  return useMutation({
    mutationFn: ({ from, to }: { from: string; to: string }) =>
      clearPlan(from, to),
    onSuccess: (res) => {
      void invalidatePlanViews(qc)
      offerUndo(res, "plan_cleared")
    },
    onError: (err) => toastError(err, t),
  })
}

/**
 * "Mark as No plate" on a board cell (keyboard N): sets a `none` override,
 * removing the cell's plate, and offers Undo for UNDO_MS. Undo puts the
 * cell's previous override back (or clears it) and restores the plate with
 * its batch from the returned snapshot. The override goes first because a
 * No plate cell refuses the plate; if the restore then fails, the cell is
 * marked No plate again so Undo never half-applies.
 */
export function useMarkNoPlate() {
  const qc = useQueryClient()
  const { t } = useTranslation()

  function refresh() {
    void invalidatePlanViews(qc)
    void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
  }

  async function undo(cell: PlanCell, removed: PlanSnapshot | null) {
    let reverted = false
    try {
      if (cell.source === "override") {
        await setAvailabilityOverride(cell.date, cell.slot_id, {
          mode: cell.mode,
          preset_id: cell.preset_id,
          note: cell.note,
        })
      } else {
        await clearAvailabilityOverride(cell.date, cell.slot_id)
      }
      reverted = true
      if (removed) await restorePlan(removed)
      toast.success(t("plan.undo.restored"))
    } catch (err) {
      if (reverted) {
        await setAvailabilityOverride(cell.date, cell.slot_id, {
          mode: "none",
          note: cell.note,
          // The toast below reports the failed Undo and the refresh shows
          // the cell as it really is.
        }).catch(() => {})
      }
      toastUndoError(err, t)
    } finally {
      refresh()
    }
  }

  return useMutation({
    mutationFn: (cell: PlanCell) =>
      setAvailabilityOverride(cell.date, cell.slot_id, {
        mode: "none",
        note: cell.note,
        remove_plate: cell.plate !== null,
      }),
    onSuccess: (res, cell) => {
      refresh()
      // Undo works once, and not after the toast has closed.
      let closed = false
      const close = () => {
        closed = true
      }
      toast(t("plan.undo.marked_no_plate"), {
        duration: UNDO_MS,
        action: {
          label: t("common.undo"),
          onClick: () => {
            if (closed) return
            closed = true
            void undo(cell, res.removed)
          },
        },
        onDismiss: close,
        onAutoClose: close,
      })
    },
    onError: (err) => toastError(err, t),
  })
}

/**
 * "Plan this meal" on a No plate cell. A date override is dropped so the
 * cell falls back to its weekday default; only when that default is `none`
 * too does it get an explicit `plan` override, so dates are not pinned
 * needlessly.
 */
export function usePlanMeal() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: async (cell: PlanCell) => {
      if (cell.source === "override") {
        await clearAvailabilityOverride(cell.date, cell.slot_id)
        const { items } = await getAvailabilityRange(cell.date, cell.date)
        const resolved = items.find((c) => c.slot_id === cell.slot_id)
        // A cell the range doesn't resolve gets the explicit override.
        if (resolved && resolved.mode !== "none") return
      }
      await setAvailabilityOverride(cell.date, cell.slot_id, { mode: "plan" })
    },
    onSuccess: () => {
      void invalidatePlanViews(qc)
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => toastError(err, t),
  })
}

/** The reason on a No plate cell ("Eating out"); empty clears it. */
export function useSetNoPlateNote() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ cell, note }: { cell: PlanCell; note: string | null }) =>
      setAvailabilityOverride(cell.date, cell.slot_id, { mode: "none", note }),
    onSuccess: () => {
      void invalidatePlanViews(qc)
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => toastError(err, t),
  })
}

// ── Draft / confirmed (D3, D4) ────────────────────────────────────────

/** A plate or a session: either confirms or unlocks its batch closure. */
export interface StateTarget {
  kind: "plate" | "session"
  id: number
}

/**
 * Confirm: the target's whole batch closure becomes confirmed, with the
 * shared undo toast (D34: no dialog). Undo unlocks the same closure.
 */
export function useConfirm() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offer = useUndoToast()
  const unlock = useUnlock()
  return useMutation({
    mutationFn: ({ kind, id }: StateTarget) =>
      kind === "plate" ? confirmPlate(id) : confirmSession(id),
    onSuccess: (rows, target) => {
      offer(t("plan.state.confirmed", { count: rows.plate_ids.length }), () =>
        unlock.mutate(target)
      )
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

/** Unlock: the target's whole batch closure returns to draft. */
export function useUnlock() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ kind, id }: StateTarget) =>
      kind === "plate" ? unlockPlate(id) : unlockSession(id),
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => toastError(err, t),
  })
}

/** Undo of accept: returns exactly the accepted rows to draft. */
export function useUnaccept() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (rows: StateChange) => unacceptPlan(rows),
    onSuccess: () => {
      toast.success(t("plan.undo.restored"))
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastUndoError(err, t),
  })
}

/**
 * Accept: confirms every draft in [from, to] and offers Undo, which
 * unaccepts exactly the rows the accept returned. Nothing accepted offers
 * nothing.
 */
export function useAccept() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offer = useUndoToast()
  const unaccept = useUnaccept()
  return useMutation({
    mutationFn: ({ from, to }: { from: string; to: string }) =>
      acceptPlan(from, to),
    onSuccess: (rows) => {
      if (rows.plate_ids.length > 0 || rows.session_ids.length > 0) {
        offer(t("plan.accepted", { count: rows.plate_ids.length }), () =>
          unaccept.mutate(rows)
        )
      }
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

/** Discard draft: deletes every draft in [from, to]; Undo restores them. */
export function useDiscard() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerUndo = useUndoableRemoval()
  return useMutation({
    mutationFn: ({ from, to }: { from: string; to: string }) =>
      discardPlan(from, to),
    onSuccess: (res) => {
      offerUndo(res, "draft_discarded")
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

// ── Mid-week re-plan (§3.2) ───────────────────────────────────────────

/** What re-planning [from, to] would touch. `enabled` false while the
 *  sheet is closed. */
export function useReplanPreview(
  from: string,
  to: string,
  noneCells: ReplanCell[] = [],
  enabled = true
) {
  return useQuery({
    queryKey: planKeys.replanPreview(from, to, noneCells),
    queryFn: () => getReplanPreview(from, to, noneCells),
    enabled,
  })
}

/** Re-plan: removes what the changed cells free up and, with
 *  `regenerate`, attaches the board to the started run (`?run=`). */
export function useReplan() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const navigate = useNavigate()
  return useMutation({
    mutationFn: (input: ReplanInput) => replan(input),
    onSuccess: async ({ run_id }) => {
      await invalidatePlanViews(qc)
      if (run_id !== null) {
        await navigate({
          to: "/plan",
          search: (prev) => ({ ...prev, run: run_id }),
        })
      }
    },
    onError: (err) => toastError(err, t),
  })
}
