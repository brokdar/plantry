import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  copyPlate,
  getAlternatives,
  moveCook,
  movePlate,
  replaceComponent,
  setAway,
  type AlsoChanged,
  type Alternative,
  type AlternativesInput,
  type AwayInput,
  type MealCell,
  type MoveInput,
  type Undo,
} from "@/lib/api/meal"
import { addPlateComponent, deletePlateComponent } from "@/lib/api/plates"
import { toastError } from "@/lib/toast"

import { mealKeys } from "./keys"
import { invalidatePlanViews, mergeUndo, useRevertUndo } from "./plan"

// Meal-level actions (meal editing slice 5). Each hook owns its invalidation
// and error toast and offers one Undo through the revert token. `describe`
// turns the response's also_changed into the toast's second line.

export type DescribeAlsoChanged = (list: AlsoChanged[]) => string | undefined

/** POST /api/plates/{id}/move, with Undo. */
export function useMovePlate(describe?: DescribeAlsoChanged) {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerRevert = useRevertUndo()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: MoveInput }) =>
      movePlate(id, input),
    onSuccess: (res, { input }) => {
      offerRevert(
        res.undo,
        t(
          input.occupant === "swap"
            ? "plan.meal.undo.swapped"
            : "plan.meal.undo.moved"
        ),
        describe?.(res.also_changed)
      )
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

/** POST /api/plates/{id}/copy, with Undo. */
export function useCopyPlate() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerRevert = useRevertUndo()
  return useMutation({
    mutationFn: ({
      id,
      cells,
      as,
    }: {
      id: number
      cells: MealCell[]
      as: "fresh" | "leftover"
    }) => copyPlate(id, cells, as),
    onSuccess: (res) => {
      offerRevert(res.undo, t("plan.meal.undo.copied"))
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

/** POST /api/cook-sessions/{id}/move-cook, with Undo; a refusal shows the
 *  backend's message. */
export function useMoveCook() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerRevert = useRevertUndo()
  return useMutation({
    mutationFn: ({ id, cell }: { id: number; cell: MealCell }) =>
      moveCook(id, cell),
    onSuccess: (res) => {
      offerRevert(res.undo, t("plan.meal.undo.cook_moved"))
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

/** POST /api/availability/away (a cell or a whole day), with Undo. */
export function useSetAway(describe?: DescribeAlsoChanged) {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerRevert = useRevertUndo()
  return useMutation({
    mutationFn: (input: AwayInput) => setAway(input),
    onSuccess: (res, input) => {
      offerRevert(
        res.undo,
        t("date" in input ? "plan.meal.undo.day_away" : "plan.meal.undo.away"),
        describe?.(res.also_changed)
      )
      void qc.invalidateQueries({ queryKey: ["availability"] })
      return invalidatePlanViews(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

/** The ranked alternatives for a plate (writes nothing). */
export function useAlternatives(
  start: string,
  input: AlternativesInput | null
) {
  return useQuery({
    queryKey: mealKeys.alternatives(start, input),
    queryFn: () => getAlternatives(start, input!),
    enabled: input !== null,
    select: (res) => res.alternatives,
  })
}

/**
 * Applies an alternative's ops as one action with one Undo. A swap, or an
 * add from a batch with the remove of what it replaces, is one replace;
 * an add alone adds; every other remove follows. The calls run in order
 * and the Undo covers the whole sequence (first to last token).
 */
export function useApplyAlternative(describe?: DescribeAlsoChanged) {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerRevert = useRevertUndo()
  return useMutation({
    mutationFn: async ({
      plateId,
      alt,
    }: {
      plateId: number
      alt: Alternative
    }) => {
      const tokens: Undo[] = []
      const also: AlsoChanged[] = []
      for (const call of alternativeCalls(plateId, alt)) {
        const res = await call()
        tokens.push(res.undo)
        also.push(...(res.also_changed ?? []))
      }
      return { undo: mergeUndo(tokens), also_changed: also }
    },
    onSuccess: (res) => {
      offerRevert(
        res.undo,
        t("plan.meal.undo.replaced"),
        describe?.(res.also_changed)
      )
      return invalidatePlanViews(qc)
    },
    onError: (err) => {
      void invalidatePlanViews(qc)
      toastError(err, t)
    },
  })
}

type Step = () => Promise<{ undo: Undo; also_changed?: AlsoChanged[] }>

/** The calls that apply an alternative's ops (exported for its test). */
export function alternativeCalls(plateId: number, alt: Alternative): Step[] {
  const steps: Step[] = []
  const ops = [...alt.ops]
  const swap = ops.find((o) => o.op === "swap")
  const add = ops.find((o) => o.op === "add")
  let replacedId: number | undefined
  if (swap) {
    replacedId = swap.plate_component_id
    steps.push(() =>
      replaceComponent(plateId, {
        plate_component_id: swap.plate_component_id,
        food_id: swap.food_id,
        ...qty(swap),
      })
    )
  } else if (add) {
    // An add from a batch replaces the component its first remove drops.
    const remove = add.session_id
      ? ops.find((o) => o.op === "remove")
      : undefined
    if (remove) {
      replacedId = remove.plate_component_id
      steps.push(() =>
        replaceComponent(plateId, {
          plate_component_id: remove.plate_component_id,
          food_id: add.food_id,
          session_id: add.session_id,
          ...qty(add),
        })
      )
    } else {
      steps.push(() =>
        addPlateComponent(
          plateId,
          add.amount != null && add.unit
            ? { food_id: add.food_id, amount: add.amount, unit: add.unit }
            : {
                food_id: add.food_id,
                portions: add.portions,
                cook_session_id: add.session_id,
              }
        )
      )
    }
  }
  for (const o of ops) {
    if (o.op !== "remove" || o.plate_component_id === replacedId) continue
    steps.push(() => deletePlateComponent(plateId, o.plate_component_id))
  }
  return steps
}

function qty(o: { portions?: number; amount?: number; unit?: string }) {
  if (o.amount != null && o.unit) return { amount: o.amount, unit: o.unit }
  if (o.portions != null) return { portions: o.portions }
  return {}
}
