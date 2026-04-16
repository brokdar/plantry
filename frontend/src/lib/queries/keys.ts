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
  variants: (id: number) => [...componentKeys.detail(id), "variants"] as const,
}

export const slotKeys = {
  all: ["slots"] as const,
  lists: () => [...slotKeys.all, "list"] as const,
  list: (activeOnly: boolean) => [...slotKeys.lists(), { activeOnly }] as const,
}

export const weekKeys = {
  all: ["weeks"] as const,
  current: () => [...weekKeys.all, "current"] as const,
  byDate: (year: number, week: number) =>
    [...weekKeys.all, "by-date", { year, week }] as const,
  byId: (id: number) => [...weekKeys.all, id] as const,
  list: (limit: number, offset: number) =>
    [...weekKeys.all, "list", { limit, offset }] as const,
}

export const plateKeys = {
  all: ["plates"] as const,
  detail: (id: number) => [...plateKeys.all, id] as const,
}

export { lookupKeys } from "./lookup"
export { portionKeys } from "./portions"
export { imageKeys } from "./images"
