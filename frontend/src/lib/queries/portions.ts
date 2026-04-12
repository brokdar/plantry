import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { listPortions, upsertPortion, deletePortion } from "@/lib/api/portions"

export const portionKeys = {
  all: ["portions"] as const,
  list: (ingredientId: number) => [...portionKeys.all, ingredientId] as const,
}

export function usePortions(ingredientId: number) {
  return useQuery({
    queryKey: portionKeys.list(ingredientId),
    queryFn: () => listPortions(ingredientId),
    enabled: ingredientId > 0,
  })
}

export function useUpsertPortion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      ingredientId,
      data,
    }: {
      ingredientId: number
      data: { unit: string; grams: number }
    }) => upsertPortion(ingredientId, data),
    onSuccess: (_, { ingredientId }) => {
      void qc.invalidateQueries({ queryKey: portionKeys.list(ingredientId) })
    },
  })
}

export function useDeletePortion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      ingredientId,
      unit,
    }: {
      ingredientId: number
      unit: string
    }) => deletePortion(ingredientId, unit),
    onSuccess: (_, { ingredientId }) => {
      void qc.invalidateQueries({ queryKey: portionKeys.list(ingredientId) })
    },
  })
}
