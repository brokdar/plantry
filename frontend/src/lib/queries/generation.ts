import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate, useRouterState } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import {
  generatePlan,
  getPlanRun,
  regeneratePlan,
  type GenerateInput,
  type PlanRun,
  type RegenerateInput,
} from "@/lib/api/generation"
import { toastError } from "@/lib/toast"

import { runKeys } from "./keys"
import { invalidatePlanViews } from "./plan"

/** How often a running generation is polled (D9). */
export const RUN_POLL_MS = 500

const isRunning = (run: PlanRun | undefined) => run?.status === "running"

const BOARD_PATH = /^\/plan\/?$/

/**
 * Opens the run's window on the board with `?run=`. Already on the board,
 * only `run` is added: the board keeps its URL (an anchored board stays
 * without `start`), so it isn't re-keyed back to its skeleton.
 */
function useOpenRun() {
  const navigate = useNavigate()
  const onBoard = useRouterState({
    select: (s) => BOARD_PATH.test(s.location.pathname),
  })
  return (start: string, run: number) =>
    onBoard
      ? navigate({ to: "/plan", search: (prev) => ({ ...prev, run }) })
      : navigate({ to: "/plan", search: { start, run } })
}

/**
 * Starts a generation and opens its window on the board with `?run=`, so
 * the board shows the progress and a reload re-attaches (D35).
 */
export function useGenerate() {
  const { t } = useTranslation()
  const openRun = useOpenRun()
  return useMutation({
    mutationFn: (input: GenerateInput) => generatePlan(input),
    onSuccess: ({ run_id }, { start }) => openRun(start, run_id),
    onError: (err) => toastError(err, t),
  })
}

/** Regenerates a scope (D6) and attaches the board to the run. */
export function useRegenerate() {
  const { t } = useTranslation()
  const openRun = useOpenRun()
  return useMutation({
    mutationFn: (input: RegenerateInput) => regeneratePlan(input),
    onSuccess: ({ run_id }, { start }) => openRun(start, run_id),
    onError: (err) => toastError(err, t),
  })
}

/**
 * A generation run's progress. Polls every RUN_POLL_MS while it is
 * `running`; a finished run (`ok`, `error`, `partial`) is final, so polling
 * stops and the plan views are invalidated once, when the run is first seen
 * finished. `id` undefined = no run.
 */
export function useRun(id: number | undefined) {
  const qc = useQueryClient()
  return useQuery({
    queryKey: runKeys.plan(id ?? 0),
    queryFn: async () => {
      const key = runKeys.plan(id!)
      const before = qc.getQueryData<PlanRun>(key)
      const run = await getPlanRun(id!)
      if (!isRunning(run) && (before === undefined || isRunning(before))) {
        await invalidatePlanViews(qc)
      }
      return run
    },
    enabled: id !== undefined,
    refetchInterval: (query) =>
      isRunning(query.state.data) ? RUN_POLL_MS : false,
    staleTime: (query) =>
      query.state.data && !isRunning(query.state.data) ? Infinity : 0,
  })
}
