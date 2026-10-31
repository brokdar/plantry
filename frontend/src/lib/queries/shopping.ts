import { useQuery } from "@tanstack/react-query"

import { getShoppingList, type ShoppingRange } from "@/lib/api/shopping"

import { shoppingKeys } from "./keys"

/** The shopping list for `range`, or the server default for the anchored
 *  window. */
export function useShoppingList(range?: ShoppingRange) {
  return useQuery({
    queryKey: shoppingKeys.list(range),
    queryFn: () => getShoppingList(range),
  })
}
