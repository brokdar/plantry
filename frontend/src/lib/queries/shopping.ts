import { useQuery } from "@tanstack/react-query"

import {
  getShoppingList,
  type ShoppingList,
  type ShoppingListItem,
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

/** The sectioned list as one name-sorted list, for the old planner's
 *  ShoppingPanel. */
export function flattenShoppingList(list: ShoppingList): {
  items: ShoppingListItem[]
} {
  const items = list.sections
    .flatMap((s) => s.items)
    .sort((a, b) => a.name.localeCompare(b.name))
  return { items }
}

/** Old planner ShoppingPanel only; deleted by F7. */
export function usePlannerShoppingList(from: string, to: string) {
  return useQuery({
    queryKey: shoppingKeys.planner(from, to),
    queryFn: () => getShoppingList({ from, to }),
    select: flattenShoppingList,
    enabled: !!from && !!to,
  })
}
