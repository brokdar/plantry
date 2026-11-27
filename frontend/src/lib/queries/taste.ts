import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import type { FoodEventUndo } from "@/lib/api/foods"
import { revertFoodChange } from "@/lib/api/foods"
import {
  confirmStatement,
  getTaste,
  makeItemNever,
  proposeStatements,
  removeItem,
  removeNever,
  setItemPolarity,
  type Statement,
  type TastePolarity,
} from "@/lib/api/taste"
import { toastError } from "@/lib/toast"

import { tasteKeys } from "./keys"
import { useUndoToast } from "./plan"

function refresh(qc: QueryClient) {
  return qc.invalidateQueries({ queryKey: tasteKeys.all })
}

export function useTaste() {
  return useQuery({ queryKey: tasteKeys.all, queryFn: () => getTaste() })
}

/** Flip, remove and make-never on the Taste page; a flip and a removal offer
 *  an Undo through the food change log. */
export function useTasteEdits() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offer = useUndoToast()
  const onError = (err: unknown) => toastError(err, t)
  const undo = useMutation({
    mutationFn: (u: FoodEventUndo) => revertFoodChange(u),
    onSuccess: () => refresh(qc),
    onError,
  })
  const done = (message: string) => (r: { undo: FoodEventUndo }) => {
    void refresh(qc)
    offer(message, () => undo.mutate(r.undo))
  }
  return {
    flip: useMutation({
      mutationFn: (v: { id: number; polarity: TastePolarity }) =>
        setItemPolarity(v.id, v.polarity),
      onSuccess: done(t("settings_page.taste.flipped")),
      onError,
    }),
    remove: useMutation({
      mutationFn: (id: number) => removeItem(id),
      onSuccess: done(t("settings_page.taste.removed")),
      onError,
    }),
    makeNever: useMutation({
      mutationFn: (id: number) => makeItemNever(id),
      onSuccess: () => refresh(qc),
      onError,
    }),
    removeNever: useMutation({
      mutationFn: (foodId: number) => removeNever(foodId),
      onSuccess: () => refresh(qc),
      onError,
    }),
    undo,
  }
}

/** The chat's statement chip: [Always] or [Just this week]. */
export function useConfirmStatement() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (v: { statement: Statement; scope: "always" | "window" }) =>
      confirmStatement(v.statement, v.scope),
    onSuccess: () => refresh(qc),
    onError: (err) => toastError(err, t),
  })
}

/** The statements a free-text answer proposes (COLD-1). */
export function useProposeStatements() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (text: string) => proposeStatements(text),
    onError: (err) => toastError(err, t),
  })
}
