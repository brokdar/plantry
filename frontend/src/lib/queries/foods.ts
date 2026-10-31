import { keepPreviousData, useQuery, useMutation } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  createFood,
  createVariant,
  deleteFood,
  deletePortion,
  getFood,
  getFoodNutrition,
  getScaled,
  listFoodMacros,
  listFoods,
  listPortions,
  listVariants,
  refetchFood,
  setDietClass,
  setFoodFavorite,
  setShopSection,
  syncPortions,
  updateFood,
  upsertPortion,
  type DietClass,
  type FoodInput,
  type LeafFoodInput,
  type FoodListParams,
} from "@/lib/api/foods"
import type { ShopSection } from "@/lib/api/shopping"
import { ApiError } from "@/lib/api/client"
import { queryClient } from "@/lib/query-client"
import { toastError } from "@/lib/toast"
import { foodKeys, planKeys, setupKeys, shoppingKeys } from "./keys"

export function useFoods(
  params?: FoodListParams,
  options?: { staleTime?: number }
) {
  return useQuery({
    queryKey: foodKeys.list(params ?? {}),
    queryFn: () => listFoods(params),
    staleTime: options?.staleTime,
  })
}

export function useFood(id: number) {
  return useQuery({
    queryKey: foodKeys.detail(id),
    queryFn: () => getFood(id),
    enabled: id > 0,
  })
}

export function useFoodNutrition(id: number) {
  return useQuery({
    queryKey: foodKeys.nutrition(id),
    queryFn: () => getFoodNutrition(id),
    enabled: id > 0,
  })
}

/** Batch per-portion (composed) or per-100g (leaf) macros for the given food
 *  ids. Pass an empty list to disable. The query key sorts ids so callers
 *  passing the same set in different order share cache. */
export function useFoodMacros(ids: readonly number[]) {
  return useQuery({
    queryKey: foodKeys.macrosBatch(ids),
    queryFn: () => listFoodMacros(ids),
    enabled: ids.length > 0,
  })
}

export function useCreateFood() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (input: FoodInput) => createFood(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      // The recipe count drives the Generate gate and the guided state.
      void queryClient.invalidateQueries({ queryKey: setupKeys.status() })
    },
    onError: (err) => toastError(err, t),
  })
}

export interface EnsuredLeaf {
  id: number
  name: string
}

/** Creates the leaf, or — when a leaf with that name already exists (409) —
 *  returns the existing one instead. */
async function createLeafOrReuse(input: LeafFoodInput): Promise<EnsuredLeaf> {
  try {
    const food = await createFood(input)
    return { id: food.id, name: food.name }
  } catch (err) {
    if (err instanceof ApiError && err.status === 409 && err.existingId) {
      return { id: err.existingId, name: input.name }
    }
    throw err
  }
}

/** Like useCreateFood for a leaf, but a duplicate name resolves to the
 *  existing leaf instead of failing (and toasting). */
export function useEnsureLeaf() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: createLeafOrReuse,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useUpdateFood() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: FoodInput }) =>
      updateFood(id, data),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({
        queryKey: foodKeys.detail(variables.id),
      })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useDeleteFood() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => deleteFood(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: setupKeys.status() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useVariants(id: number) {
  return useQuery({
    queryKey: foodKeys.variants(id),
    queryFn: () => listVariants(id),
    enabled: id > 0,
  })
}

/** A recipe's ingredients scaled to `portions`. Keeps showing the previous
 *  list while a new portion count loads, so a stepper doesn't flash. */
export function useScaled(foodId: number, portions: number) {
  return useQuery({
    queryKey: foodKeys.scaled(foodId, portions),
    queryFn: () => getScaled(foodId, portions),
    enabled: foodId > 0 && portions > 0,
    placeholderData: keepPreviousData,
  })
}

/** Moves a leaf to a store section; `null` reverts to the classifier. */
export function useSetShopSection() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({
      id,
      section,
    }: {
      id: number
      section: ShopSection | null
    }) => setShopSection(id, section),
    onSuccess: (_data, { id }) => {
      void queryClient.invalidateQueries({ queryKey: shoppingKeys.all })
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: foodKeys.detail(id) })
    },
    onError: (err) => toastError(err, t),
  })
}

/** Overrides a food's diet class; `null` drops the override (D28). Every
 *  recipe above the food derives its class from it, and the plan's diet
 *  issues (H5) read it, so all foods and the plan refetch. */
export function useSetDietClass() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({
      id,
      dietClass,
    }: {
      id: number
      dietClass: DietClass | null
    }) => setDietClass(id, dietClass),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: foodKeys.all }),
        queryClient.invalidateQueries({ queryKey: planKeys.all }),
      ]),
    onError: (err) => toastError(err, t),
  })
}

export function useSetFoodFavorite() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, favorite }: { id: number; favorite: boolean }) =>
      setFoodFavorite(id, favorite),
    onSuccess: (_data, { id }) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: foodKeys.detail(id) })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useCreateVariant() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => createVariant(id),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.variants(id) })
      void queryClient.invalidateQueries({ queryKey: foodKeys.detail(id) })
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: setupKeys.status() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useSyncPortions() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => syncPortions(id),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.portions(id) })
      void queryClient.invalidateQueries({ queryKey: foodKeys.detail(id) })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useRefetchFood() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, lang }: { id: number; lang?: string }) =>
      refetchFood(id, lang),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({
        queryKey: foodKeys.detail(variables.id),
      })
    },
    onError: (err) => toastError(err, t),
  })
}

export function usePortions(foodId: number) {
  return useQuery({
    queryKey: foodKeys.portions(foodId),
    queryFn: () => listPortions(foodId),
    enabled: foodId > 0,
  })
}

export function useUpsertPortion() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({
      foodId,
      data,
    }: {
      foodId: number
      data: { unit: string; grams: number }
    }) => upsertPortion(foodId, data),
    onSuccess: (_, { foodId }) => {
      void queryClient.invalidateQueries({
        queryKey: foodKeys.portions(foodId),
      })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useDeletePortion() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ foodId, unit }: { foodId: number; unit: string }) =>
      deletePortion(foodId, unit),
    onSuccess: (_, { foodId }) => {
      void queryClient.invalidateQueries({
        queryKey: foodKeys.portions(foodId),
      })
    },
    onError: (err) => toastError(err, t),
  })
}
