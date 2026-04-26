import { useQuery } from "@tanstack/react-query"

import { getNutritionRange } from "@/lib/api/nutrition"

export const nutritionKeys = {
  range: (from: string, to: string) =>
    ["nutrition", "range", from, to] as const,
}

export function useNutritionRange(from: string, to: string) {
  return useQuery({
    queryKey: nutritionKeys.range(from, to),
    queryFn: () => getNutritionRange(from, to),
    enabled: !!from && !!to,
  })
}
