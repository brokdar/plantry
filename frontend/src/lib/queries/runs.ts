import { keepPreviousData, useQuery } from "@tanstack/react-query"

import { getInsights, type InsightsParams } from "@/lib/api/insights"
import { getPrice } from "@/lib/api/prices"
import { getRun, listRuns, type RunsParams } from "@/lib/api/runs"

import { insightKeys, runKeys } from "./keys"

/** One page of the run log; the previous page stays while the next loads. */
export function useRuns(params: RunsParams = {}) {
  return useQuery({
    queryKey: runKeys.list(params),
    queryFn: () => listRuns(params),
    placeholderData: keepPreviousData,
  })
}

/** A run's detail (steps, prompt); `id` undefined = none open. */
export function useRunDetail(id: number | undefined) {
  return useQuery({
    queryKey: runKeys.detail(id ?? 0),
    queryFn: () => getRun(id!),
    enabled: id !== undefined,
  })
}

/** Weekly trends and the by-prompt-version table. */
export function useRunInsights(params: InsightsParams = {}) {
  return useQuery({
    queryKey: insightKeys.summary(params),
    queryFn: () => getInsights(params),
  })
}

/** The effective price of `model`; disabled without a model. */
export function useModelPrice(model: string) {
  return useQuery({
    queryKey: insightKeys.price(model),
    queryFn: () => getPrice(model),
    enabled: model !== "",
  })
}
