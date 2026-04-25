import { useQuery, useMutation } from "@tanstack/react-query"
import {
  createFood,
  createVariant,
  deleteFood,
  deletePortion,
  getFood,
  getFoodNutrition,
  getInsights,
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
import { foodKeys } from "./keys"

export function useFoods(params?: FoodListParams) {
  return useQuery({
    queryKey: foodKeys.list(params ?? {}),
    queryFn: () => listFoods(params),
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

export function useCreateFood() {
  return useMutation({
    mutationFn: (input: FoodInput) => createFood(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
    },
  })
}

export function useUpdateFood() {
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: FoodInput }) =>
      updateFood(id, data),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({
        queryKey: foodKeys.detail(variables.id),
      })
    },
  })
}

export function useDeleteFood() {
  return useMutation({
    mutationFn: (id: number) => deleteFood(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
    },
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
  return useMutation({
    mutationFn: ({ id, favorite }: { id: number; favorite: boolean }) =>
      setFoodFavorite(id, favorite),
    onSuccess: (_data, { id }) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: foodKeys.detail(id) })
    },
  })
}

export function useCreateVariant() {
  return useMutation({
    mutationFn: (id: number) => createVariant(id),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.variants(id) })
      void queryClient.invalidateQueries({ queryKey: foodKeys.detail(id) })
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
    },
  })
}

export function useSyncPortions() {
  return useMutation({
    mutationFn: (id: number) => syncPortions(id),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.portions(id) })
      void queryClient.invalidateQueries({ queryKey: foodKeys.detail(id) })
    },
  })
}

export function useRefetchFood() {
  return useMutation({
    mutationFn: ({ id, lang }: { id: number; lang?: string }) =>
      refetchFood(id, lang),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: foodKeys.lists() })
      void queryClient.invalidateQueries({
        queryKey: foodKeys.detail(variables.id),
      })
    },
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
  })
}

export function useDeletePortion() {
  return useMutation({
    mutationFn: ({ foodId, unit }: { foodId: number; unit: string }) =>
      deletePortion(foodId, unit),
    onSuccess: (_, { foodId }) => {
      void queryClient.invalidateQueries({
        queryKey: foodKeys.portions(foodId),
      })
    },
  })
}
