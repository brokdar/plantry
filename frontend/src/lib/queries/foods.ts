import { useQuery, useMutation } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  createFood,
  createVariant,
  deleteFood,
  deletePortion,
  getFood,
  getFoodNutrition,
  getInsights,
  listFoodMacros,
  listFoods,
  listPortions,
  listVariants,
  refetchFood,
  setFoodFavorite,
  syncPortions,
  updateFood,
  upsertPortion,
  type FoodInput,
  type FoodListParams,
  type InsightsParams,
} from "@/lib/api/foods"
import { queryClient } from "@/lib/query-client"
import { toastError } from "@/lib/toast"
import { foodKeys } from "./keys"

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

export function useInsights(params?: InsightsParams) {
  return useQuery({
    queryKey: foodKeys.insights(params ?? {}),
    queryFn: () => getInsights(params),
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
