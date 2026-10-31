import type { ShoppingRange } from "@/lib/api/shopping"

export const foodKeys = {
  all: ["foods"] as const,
  lists: () => [...foodKeys.all, "list"] as const,
  list: (params: object) => [...foodKeys.lists(), params] as const,
  details: () => [...foodKeys.all, "detail"] as const,
  detail: (id: number) => [...foodKeys.details(), id] as const,
  nutrition: (id: number) => [...foodKeys.detail(id), "nutrition"] as const,
  variants: (id: number) => [...foodKeys.detail(id), "variants"] as const,
  portions: (id: number) => [...foodKeys.detail(id), "portions"] as const,
  // Under detail(id): a food update also drops its scaled lists.
  scaled: (id: number, portions: number) =>
    [...foodKeys.detail(id), "scaled", portions] as const,
  // Batch per-food macros, keyed by a stable comma-joined id list. Mirrors
  // foodKeys.all so cache invalidations on a food update also drop the
  // batched macros entries that referenced it.
  macrosBatch: (ids: readonly number[]) =>
    [
      ...foodKeys.all,
      "macros-batch",
      ids.slice().sort((a, b) => a - b),
    ] as const,
}

export const slotKeys = {
  all: ["slots"] as const,
  lists: () => [...slotKeys.all, "list"] as const,
  list: (activeOnly: boolean) => [...slotKeys.lists(), { activeOnly }] as const,
}

// Read models (D23). Every plan mutation invalidates planKeys.all,
// todayKeys.all and shoppingKeys.all.
export const planKeys = {
  all: ["plan"] as const,
  /** `start` undefined = the anchored window. */
  window: (start?: string) => [...planKeys.all, "window", start ?? ""] as const,
}

export const todayKeys = {
  all: ["today"] as const,
  date: (date: string) => [...todayKeys.all, date] as const,
}

export const sessionKeys = {
  all: ["sessions"] as const,
  detail: (id: number) => [...sessionKeys.all, id] as const,
  // Under detail(id): a session mutation also drops its leftover cells.
  leftoverCells: (id: number, count: number) =>
    [...sessionKeys.detail(id), "leftover-cells", count] as const,
}

export const plateKeys = {
  all: ["plates"] as const,
  detail: (id: number) => [...plateKeys.all, id] as const,
  range: (from: string, to: string) =>
    [...plateKeys.all, "range", from, to] as const,
  rangeInfinite: (anchor: string) =>
    [...plateKeys.all, "range-infinite", anchor] as const,
  byDate: (date: string) => [...plateKeys.all, "by-date", date] as const,
  // Sits UNDER `range(from, to)` so that any invalidation of the same
  // window's plate list also drops the macros for that window. TanStack
  // Query treats the queryKey as a prefix list, so `range(...)` matches
  // both `["plates", "range", from, to]` and the deeper macros key below.
  macrosRange: (from: string, to: string) =>
    [...plateKeys.range(from, to), "macros"] as const,
}

export const availabilityKeys = {
  all: ["availability"] as const,
  defaults: () => [...availabilityKeys.all, "defaults"] as const,
  ranges: () => [...availabilityKeys.all, "range"] as const,
  range: (from: string, to: string) =>
    [...availabilityKeys.ranges(), from, to] as const,
}

export const profileKeys = {
  detail: ["profile"] as const,
}

export const presetKeys = {
  all: ["presets"] as const,
  lists: () => [...presetKeys.all, "list"] as const,
  list: (params: object) => [...presetKeys.lists(), params] as const,
  details: () => [...presetKeys.all, "detail"] as const,
  detail: (id: number) => [...presetKeys.details(), id] as const,
  knownTags: () => [...presetKeys.all, "known-tags"] as const,
}

export const aiKeys = {
  all: ["ai"] as const,
  conversations: () => [...aiKeys.all, "conversations"] as const,
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

export const setupKeys = {
  all: ["setup"] as const,
  status: () => [...setupKeys.all, "status"] as const,
}

export const shoppingKeys = {
  all: ["shopping"] as const,
  /** `range` undefined = the server default for the anchored window. */
  list: (range?: ShoppingRange) =>
    [...shoppingKeys.all, "list", range ?? {}] as const,
  /** The old planner's `{items}` list; F7 deletes it with ShoppingPanel. */
  planner: (from: string, to: string) =>
    [...shoppingKeys.all, "planner", from, to] as const,
}

export const importKeys = {
  all: ["import"] as const,
  lineLookup: (query: string) =>
    [...importKeys.all, "line-lookup", query] as const,
}

export const unitKeys = {
  list: () => ["units"] as const,
}

export { lookupKeys } from "./lookup"
