import { useQuery } from "@tanstack/react-query"

import { getShoppingList } from "@/lib/api/shopping"

export const shoppingKeys = {
  all: ["shopping"] as const,
  list: (from: string, to: string) => ["shopping", "list", from, to] as const,
}

export function useShoppingList(from: string, to: string) {
  return useQuery({
    queryKey: shoppingKeys.list(from, to),
    queryFn: () => getShoppingList(from, to),
    enabled: !!from && !!to,
  })
}
