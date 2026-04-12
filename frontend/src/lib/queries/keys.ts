export const ingredientKeys = {
  all: ["ingredients"] as const,
  lists: () => [...ingredientKeys.all, "list"] as const,
  list: (params: object) => [...ingredientKeys.lists(), params] as const,
  details: () => [...ingredientKeys.all, "detail"] as const,
  detail: (id: number) => [...ingredientKeys.details(), id] as const,
}

export { lookupKeys } from "./lookup"
export { portionKeys } from "./portions"
