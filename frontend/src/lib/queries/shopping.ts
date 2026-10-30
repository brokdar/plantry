import { useQuery } from "@tanstack/react-query"

import {
  getPlannerShoppingList,
  getShoppingList,
  type ShoppingRange,
} from "@/lib/api/shopping"

import { shoppingKeys } from "./keys"

/** The shopping list for `range`, or the server default for the anchored
 *  window. */
export function useShoppingList(range?: ShoppingRange) {
  return useQuery({
    queryKey: shoppingKeys.list(range),
    queryFn: () => getShoppingList(range),
  })
}

/** Old planner ShoppingPanel only; deleted by F7. */
export function usePlannerShoppingList(from: string, to: string) {
  return useQuery({
    queryKey: shoppingKeys.planner(from, to),
    queryFn: () => getPlannerShoppingList(from, to),
    enabled: !!from && !!to,
  })
}
