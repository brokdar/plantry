import {
  keepPreviousData,
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
import { revertPlan, type Undo } from "@/lib/api/meal"
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

import {
  aiKeys,
  availabilityKeys,
  planKeys,
  shoppingKeys,
  todayKeys,
} from "./keys"

/** Every plan mutation refreshes the read models and the shopping list.
 *  Returned from `onSuccess`, the mutation stays pending until the active
 *  views have refetched, so a control showing a pending value never flashes
 *  back to the old one. It never rejects: the change is already saved, so
 *  a failed refetch must not turn the mutation into an error (each view
 *  shows its own load error). */
export function invalidatePlanViews(qc: QueryClient): Promise<unknown> {
  return Promise.all([
    qc.invalidateQueries({ queryKey: planKeys.all }),
    qc.invalidateQueries({ queryKey: todayKeys.all }),
    qc.invalidateQueries({ queryKey: shoppingKeys.all }),
  ]).catch(() => undefined)
}

/** The plan views plus the assistant's conversations: accepting or
 *  unaccepting rows changes which of their change lines can be undone. */
function invalidateWithAssistant(qc: QueryClient): Promise<unknown> {
  return Promise.all([
    invalidatePlanViews(qc),
    qc.invalidateQueries({ queryKey: aiKeys.all }).catch(() => undefined),
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
  return function offer(
    message: string,
    undo: () => void,
    description?: string
  ) {
    let closed = false
    const close = () => {
      closed = true
    }
    return toast(message, {
      description,
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

/** A revert token that undoes nothing (no events, no cells). */
export function emptyUndo(undo: Undo | undefined | null): boolean {
  return !undo || (undo.first_event_id === 0 && undo.cells.length === 0)
}

/** One token for several edits made in a row: the event range from the
 *  first to the last, and every cell. */
export function mergeUndo(tokens: Undo[]): Undo {
  const real = tokens.filter((u) => u.first_event_id > 0)
  return {
    first_event_id: real[0]?.first_event_id ?? 0,
    last_event_id: real.at(-1)?.last_event_id ?? 0,
    cells: tokens.flatMap((u) => u.cells),
  }
}

/**
 * The revert-token form of the shared undo toast (hand edits since meal
 * editing 2b): `offerRevert(undo, message, description?)` shows the undo
 * toast and Undo calls `POST /api/plan/revert`. A stale token (409
 * `error.plan.change_stale`) shows its message. An empty token offers
 * nothing.
 */
export function useRevertUndo() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offer = useUndoToast()
  const revert = useMutation({
    mutationFn: (undo: Undo) => revertPlan(undo),
    onSuccess: () => {
      toast.success(t("plan.undo.restored"))
      return invalidatePlanViews(qc)
    },
    onError: (err) => {
      void invalidatePlanViews(qc)
      toastUndoError(err, t)
    },
  })
  return function offerRevert(
    undo: Undo | undefined,
    message: string,
    description?: string
  ) {
    if (!undo || emptyUndo(undo)) return
    offer(message, () => revert.mutate(undo), description)
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
 * "Plan this meal" on a No plate cell. A date override is dropped so the
 * cell falls back to its weekday default; only when that default is `none`
 * too does it get an explicit `plan` override, so dates are not pinned
 * needlessly.
 */
export function usePlanMeal() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: planCell,
    onSuccess: () => {
      void invalidatePlanViews(qc)
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => toastError(err, t),
  })
}

/** "Plan this day" in the day header: every No plate cell of the day goes
 *  back to Plan, as "Plan this meal" does per cell. */
export function usePlanDay() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    // The calls are sequential and not rolled back: when one fails, the cells
    // before it stay set to Plan and the result says how many.
    mutationFn: async (cells: PlanCell[]) => {
      const todo = cells.filter((c) => c.mode === "none")
      let done = 0
      try {
        for (const c of todo) {
          await planCell(c)
          done++
        }
      } catch (err) {
        if (done === 0) throw err
        return { done, total: todo.length, failure: err }
      }
      return { done, total: todo.length, failure: undefined }
    },
    onSuccess: (res) => {
      if (res.failure !== undefined) {
        toastError(res.failure, t)
        toast.error(
          t("plan.meal.day_partial", { done: res.done, total: res.total })
        )
      }
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
      return invalidatePlanViews(qc)
    },
    onError: (err) => {
      void invalidatePlanViews(qc)
      toastError(err, t)
    },
  })
}

async function planCell(cell: PlanCell) {
  if (cell.source === "override") {
    await clearAvailabilityOverride(cell.date, cell.slot_id)
    const { items } = await getAvailabilityRange(cell.date, cell.date)
    const resolved = items.find((c) => c.slot_id === cell.slot_id)
    // A cell the range doesn't resolve gets the explicit override.
    if (resolved && resolved.mode !== "none") return
  }
  await setAvailabilityOverride(cell.date, cell.slot_id, { mode: "plan" })
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
      return invalidateWithAssistant(qc)
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
      return invalidateWithAssistant(qc)
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
 *  sheet is closed; a changed cell choice keeps the last preview shown
 *  until the new one arrives. */
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
    placeholderData: keepPreviousData,
  })
}

/** Re-plan: marks the cells No plate, removes what they free up and, with
 *  `regenerate`, attaches the board to the started run (`?run=`). */
export function useReplan() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const navigate = useNavigate()
  return useMutation({
    mutationFn: (input: ReplanInput) => replan(input),
    onSuccess: async ({ run_id }) => {
      // The No-plate cells are availability overrides.
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
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
