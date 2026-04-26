export const foodKeys = {
  all: ["foods"] as const,
  lists: () => [...foodKeys.all, "list"] as const,
  list: (params: object) => [...foodKeys.lists(), params] as const,
  details: () => [...foodKeys.all, "detail"] as const,
  detail: (id: number) => [...foodKeys.details(), id] as const,
  nutrition: (id: number) => [...foodKeys.detail(id), "nutrition"] as const,
  variants: (id: number) => [...foodKeys.detail(id), "variants"] as const,
  portions: (id: number) => [...foodKeys.detail(id), "portions"] as const,
  insights: (params: object) => [...foodKeys.all, "insights", params] as const,
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
  shoppingList: (id: number) => [...weekKeys.all, id, "shopping-list"] as const,
  nutrition: (id: number) => [...weekKeys.all, id, "nutrition"] as const,
}

export const plateKeys = {
  all: ["plates"] as const,
  detail: (id: number) => [...plateKeys.all, id] as const,
  range: (from: string, to: string) =>
    [...plateKeys.all, "range", from, to] as const,
  rangeInfinite: (anchor: string) =>
    [...plateKeys.all, "range-infinite", anchor] as const,
  byDate: (date: string) => [...plateKeys.all, "by-date", date] as const,
}

export const profileKeys = {
  detail: ["profile"] as const,
}

export const templateKeys = {
  all: ["templates"] as const,
  lists: () => [...templateKeys.all, "list"] as const,
  details: () => [...templateKeys.all, "detail"] as const,
  detail: (id: number) => [...templateKeys.details(), id] as const,
}

export const aiKeys = {
  all: ["ai"] as const,
  conversations: (weekId?: number) =>
    [...aiKeys.all, "conversations", { weekId }] as const,
  conversation: (id: number) => [...aiKeys.all, "conversation", id] as const,
  settings: () => [...aiKeys.all, "settings"] as const,
}

export const settingsKeys = {
  all: ["settings"] as const,
  list: () => [...settingsKeys.all, "list"] as const,
  system: () => [...settingsKeys.all, "system"] as const,
  aiModels: (provider: string) =>
    [...settingsKeys.all, "ai", "models", provider] as const,
}

export const importKeys = {
  all: ["import"] as const,
  lineLookup: (query: string) =>
    [...importKeys.all, "line-lookup", query] as const,
}

export { lookupKeys } from "./lookup"
