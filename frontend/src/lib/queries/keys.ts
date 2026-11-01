import type { ReplanCell } from "@/lib/api/plan"
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
  /** Under all: every plan mutation refreshes an open re-plan preview. */
  replanPreview: (from: string, to: string, noneCells: ReplanCell[] = []) =>
    [...planKeys.all, "replan-preview", from, to, noneCells] as const,
}

export const todayKeys = {
  all: ["today"] as const,
  /** `today` is the real date: the view (rate question, banners) moves
   *  with it even for the same `date`. */
  date: (date: string, today: string) =>
    [...todayKeys.all, date, today] as const,
}

export const sessionKeys = {
  all: ["sessions"] as const,
  detail: (id: number) => [...sessionKeys.all, id] as const,
  // Under detail(id): a session mutation also drops its leftover cells.
  leftoverCells: (id: number, count: number) =>
    [...sessionKeys.detail(id), "leftover-cells", count] as const,
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
}

export const setupKeys = {
  all: ["setup"] as const,
  status: () => [...setupKeys.all, "status"] as const,
}

export const shoppingKeys = {
  all: ["shopping"] as const,
  /** `range` undefined = the server default for the anchored window. */
  list: (range?: ShoppingRange, includeDrafts = false) =>
    [...shoppingKeys.all, "list", range ?? {}, { includeDrafts }] as const,
}

export const importKeys = {
  all: ["import"] as const,
  lineLookup: (query: string) =>
    [...importKeys.all, "line-lookup", query] as const,
}

// Generation runs (`/api/plan/runs/{id}`, polled) and the run log
// (`/api/ai/runs`).
export const runKeys = {
  all: ["runs"] as const,
  plan: (id: number) => [...runKeys.all, "plan", id] as const,
  lists: () => [...runKeys.all, "list"] as const,
  list: (params: object) => [...runKeys.lists(), params] as const,
  detail: (id: number) => [...runKeys.all, "detail", id] as const,
}

// Insights and prices. A price override re-prices runs on read, so it
// invalidates insightKeys.all and runKeys.all.
export const insightKeys = {
  all: ["insights"] as const,
  summary: (params: object) => [...insightKeys.all, "summary", params] as const,
  price: (model: string) => [...insightKeys.all, "price", model] as const,
}

export const unitKeys = {
  list: () => ["units"] as const,
}
