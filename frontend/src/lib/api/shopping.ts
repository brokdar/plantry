import { apiFetch } from "./client"

export interface ShoppingItem {
  food_id: number
  name: string
  total_grams: number
}

export interface ShoppingListResponse {
  items: ShoppingItem[]
}

export async function getShoppingList(
  from: string,
  to: string
): Promise<ShoppingListResponse> {
  return apiFetch(
    `/shopping-list?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  )
}
