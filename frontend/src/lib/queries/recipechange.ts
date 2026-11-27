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
  askAboutRecipe,
  checkRecipe,
  getRecipeChanges,
  saveRecipeEdit,
  type RecipeProposal,
} from "@/lib/api/recipechange"
import { toastError } from "@/lib/toast"

import { foodKeys } from "./keys"
import { useUndoToast } from "./plan"

/** A recipe change touches the page, the library and the variant lists. */
function invalidateFoods(qc: QueryClient) {
  return qc.invalidateQueries({ queryKey: foodKeys.all })
}

/** The recipe's changes list. */
export function useRecipeChanges(id: number) {
  return useQuery({
    queryKey: foodKeys.changes(id),
    queryFn: () => getRecipeChanges(id),
  })
}

/** "Check this recipe": no model call. */
export function useCheckRecipe(id: number) {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: () => checkRecipe(id),
    onError: (err) => toastError(err, t),
  })
}

/** Undoes a recipe change through the food change log. */
export function useRevertRecipeChange() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (undo: FoodEventUndo) => revertFoodChange(undo),
    onSuccess: () => invalidateFoods(qc),
    onError: (err) => toastError(err, t),
  })
}

/** Asks about the recipe; a saved variant offers the shared Undo toast. */
export function useAskRecipe(id: number) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const offerUndo = useUndoToast()
  const revert = useRevertRecipeChange()
  return useMutation({
    mutationFn: (request: string) => askAboutRecipe(id, request),
    onSuccess: (res) => {
      if (res.kind === "variant" && res.variant && res.variant.food_id > 0) {
        const undo = res.variant.undo
        void invalidateFoods(qc)
        offerUndo(t("recipe_chat.variant_saved"), () => revert.mutate(undo))
      }
    },
    onError: (err) => toastError(err, t),
  })
}

/** Saves the confirmed in-place edit; a save offers Undo. */
export function useSaveRecipeEdit(id: number) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const offerUndo = useUndoToast()
  const revert = useRevertRecipeChange()
  return useMutation({
    mutationFn: (proposal: RecipeProposal) => saveRecipeEdit(id, proposal),
    onSuccess: (res) => {
      if (res.food_id > 0) {
        const undo = res.undo
        void invalidateFoods(qc)
        offerUndo(t("recipe_chat.edit_saved"), () => revert.mutate(undo))
      }
    },
    onError: (err) => toastError(err, t),
  })
}
