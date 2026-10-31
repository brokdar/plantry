import { useQuery } from "@tanstack/react-query"

import { getShoppingList, type ShoppingRange } from "@/lib/api/shopping"

import { shoppingKeys } from "./keys"

/** The shopping list for `range`, or the server default for the anchored
 *  window; confirmed rows only unless `includeDrafts`. */
export function useShoppingList(range?: ShoppingRange, includeDrafts = false) {
  return useQuery({
    queryKey: shoppingKeys.list(range, includeDrafts),
    queryFn: () => getShoppingList(range, includeDrafts),
  })
}
