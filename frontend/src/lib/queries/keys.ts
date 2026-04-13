export const ingredientKeys = {
  all: ["ingredients"] as const,
  lists: () => [...ingredientKeys.all, "list"] as const,
  list: (params: object) => [...ingredientKeys.lists(), params] as const,
  details: () => [...ingredientKeys.all, "detail"] as const,
  detail: (id: number) => [...ingredientKeys.details(), id] as const,
}

export const componentKeys = {
  all: ["components"] as const,
  lists: () => [...componentKeys.all, "list"] as const,
  list: (params: object) => [...componentKeys.lists(), params] as const,
  details: () => [...componentKeys.all, "detail"] as const,
  detail: (id: number) => [...componentKeys.details(), id] as const,
  nutrition: (id: number) =>
    [...componentKeys.detail(id), "nutrition"] as const,
}

export { lookupKeys } from "./lookup"
export { portionKeys } from "./portions"
export { imageKeys } from "./images"
