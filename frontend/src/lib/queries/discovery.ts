import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { useEffect } from "react"
import { useTranslation } from "react-i18next"

import {
  acceptSuggestion,
  anotherSuggestion,
  discover,
  getDailySuggestion,
  getDiscoveryRun,
  listSuggestions,
  rejectSuggestion,
  type DailyResult,
  type DiscoverInput,
  type DiscoveryRun,
  type RejectReason,
  type SuggestionsParams,
} from "@/lib/api/discovery"
import { toastError } from "@/lib/toast"

import { discoveryKeys, foodKeys, insightKeys } from "./keys"

/** How often a running discovery run is polled. */
export const DISCOVERY_POLL_MS = 1000

const NONE: DailyResult = { kind: "none" }

const isRunning = (run: DiscoveryRun | undefined) => run?.status === "running"

/** A run's progress; polls every second while `running`. `id` undefined =
 *  no run. A finished run is final. */
export function useDiscoveryRun(id: number | undefined) {
  return useQuery({
    queryKey: discoveryKeys.run(id ?? 0),
    queryFn: () => getDiscoveryRun(id!),
    enabled: id !== undefined,
    // A failed poll (e.g. 404) stops polling: the old "running" data would
    // otherwise keep it going forever.
    refetchInterval: (query) =>
      query.state.status !== "error" && isRunning(query.state.data)
        ? DISCOVERY_POLL_MS
        : false,
    staleTime: (query) =>
      query.state.data && !isRunning(query.state.data) ? Infinity : 0,
  })
}

/**
 * The daily idea for `date`. Fetched once per date; a 202 starts polling the
 * run (and only then), and when the run is done the daily endpoint is asked
 * again. Data is a `DailyResult`: a suggestion, a running run, or none (204:
 * silent, nothing to show).
 */
export function useDailySuggestion(date: string, enabled: boolean) {
  const daily = useQuery({
    queryKey: discoveryKeys.daily(date),
    queryFn: () => getDailySuggestion(date),
    enabled,
    staleTime: Infinity,
  })
  const runId = daily.data?.kind === "running" ? daily.data.runId : undefined
  const run = useDiscoveryRun(runId)
  const runDone = run.data !== undefined && !isRunning(run.data)
  const { refetch } = daily
  useEffect(() => {
    if (runId !== undefined && runDone) void refetch()
  }, [runId, runDone, refetch])
  // A run that can't be read is silent: nothing to show (UX §5.9).
  if (run.isError) return { ...daily, data: NONE }
  return daily
}

/** Starts a discovery run; the caller follows `run_id` with
 *  `useDiscoveryRun`. */
export function useDiscover() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (input: DiscoverInput) => discover(input),
    onError: (err) => toastError(err, t),
  })
}

/** Suggestions by status and/or run. Pass `enabled: false` to wait. */
export function useSuggestions(params: SuggestionsParams = {}, enabled = true) {
  return useQuery({
    queryKey: discoveryKeys.suggestionList(params),
    queryFn: () => listSuggestions(params),
    enabled,
  })
}

/** Replaces the daily idea for `date`; the answer (new suggestion, running
 *  run, or none) goes straight into the daily query. */
export function useAnotherSuggestion() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (date: string) => anotherSuggestion(date),
    onSuccess: (result: DailyResult, date) => {
      qc.setQueryData(discoveryKeys.daily(date), result)
      void qc.invalidateQueries({ queryKey: discoveryKeys.suggestions() })
    },
    onError: (err) => toastError(err, t),
  })
}

function invalidateDecided(qc: QueryClient) {
  void qc.invalidateQueries({ queryKey: discoveryKeys.all })
  void qc.invalidateQueries({ queryKey: foodKeys.lists() })
  void qc.invalidateQueries({ queryKey: insightKeys.all })
}

/** Links a suggestion to the food saved from its review. */
export function useAcceptSuggestion() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, foodId }: { id: number; foodId: number }) =>
      acceptSuggestion(id, foodId),
    onSuccess: () => invalidateDecided(qc),
    onError: (err) => toastError(err, t),
  })
}

export function useRejectSuggestion() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason?: RejectReason }) =>
      rejectSuggestion(id, reason),
    onSuccess: () => invalidateDecided(qc),
    onError: (err) => toastError(err, t),
  })
}
