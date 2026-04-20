import { useQuery, useMutation } from "@tanstack/react-query"
import {
  listIngredients,
  getIngredient,
  createIngredient,
  updateIngredient,
  deleteIngredient,
  refetchIngredient,
  type IngredientListParams,
  type IngredientInput,
} from "@/lib/api/ingredients"
import { queryClient } from "@/lib/query-client"
import { ingredientKeys } from "./keys"

export function useIngredients(params?: IngredientListParams) {
  return useQuery({
    queryKey: ingredientKeys.list(params ?? {}),
    queryFn: () => listIngredients(params),
  })
}

export function useIngredient(id: number) {
  return useQuery({
    queryKey: ingredientKeys.detail(id),
    queryFn: () => getIngredient(id),
    enabled: id > 0,
  })
}

export function useCreateIngredient() {
  return useMutation({
    mutationFn: (input: IngredientInput) => createIngredient(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ingredientKeys.lists() })
    },
  })
}

export function useUpdateIngredient() {
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: IngredientInput }) =>
      updateIngredient(id, data),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ingredientKeys.lists() })
      void queryClient.invalidateQueries({
        queryKey: ingredientKeys.detail(variables.id),
      })
    },
  })
}

export function useDeleteIngredient() {
  return useMutation({
    mutationFn: (id: number) => deleteIngredient(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ingredientKeys.lists() })
    },
  })
}

export function useRefetchIngredient() {
  return useMutation({
    mutationFn: ({ id, lang }: { id: number; lang?: string }) =>
      refetchIngredient(id, lang),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ingredientKeys.lists() })
      void queryClient.invalidateQueries({
        queryKey: ingredientKeys.detail(variables.id),
      })
    },
  })
}
