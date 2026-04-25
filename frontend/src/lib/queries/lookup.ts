import { useQuery } from "@tanstack/react-query"
import { lookupFoods, type LookupParams } from "@/lib/api/lookup"

export const lookupKeys = {
  all: ["lookup"] as const,
  search: (params: LookupParams) => [...lookupKeys.all, params] as const,
}

export function useLookup(params: LookupParams) {
  return useQuery({
    queryKey: lookupKeys.search(params),
    queryFn: () => lookupFoods(params),
    enabled: !!(params.barcode || params.query),
    staleTime: 60_000,
  })
}
