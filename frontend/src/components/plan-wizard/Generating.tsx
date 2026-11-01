import { Check, Circle, LoaderCircle } from "lucide-react"
import { useEffect, useEffectEvent } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { PlanRun, RunStage } from "@/lib/api/generation"
import { useRun } from "@/lib/queries/generation"
import { cn } from "@/lib/utils"

/** The checklist (UX §5.4). A repair pass is part of "Checking". */
const STEPS = ["resolving", "choosing", "checking", "saving"] as const

function stepIndex(stage: RunStage | null | undefined) {
  if (!stage) return 0
  if (stage === "repairing") return STEPS.indexOf("checking")
  return STEPS.indexOf(stage)
}

interface GeneratingProps {
  runId: number
  /** The run finished with a plan (`ok` or `partial`). */
  onDone: (run: PlanRun) => void
  onRetry: () => void
  /** Starts a heuristic run (D11: there is no automatic fallback). */
  onWithoutAssistant: () => void
  /** A new run is being started. */
  pending: boolean
}

/**
 * Plan wizard step 3 (UX §5.4, D35): the run's real stages as a checklist
 * while it runs. On failure it says so and offers Try again; when the
 * assistant chose, also "Plan without the assistant".
 */
export function Generating({
  runId,
  onDone,
  onRetry,
  onWithoutAssistant,
  pending,
}: GeneratingProps) {
  const { t } = useTranslation()
  const run = useRun(runId)
  const data = run.data

  const done = useEffectEvent((finished: PlanRun) => onDone(finished))
  useEffect(() => {
    if (data && (data.status === "ok" || data.status === "partial")) {
      done(data)
    }
  }, [data])

  if (run.isError || data?.status === "error") {
    const assistant = data?.chooser === "llm"
    const detail = data?.error_key
      ? t(data.error_key, { defaultValue: "" })
      : ""
    return (
      <div className="flex flex-col gap-4" data-testid="wizard-failed">
        <div role="alert" className="flex flex-col gap-1">
          <p className="text-body text-ink">
            {assistant
              ? t("plan.wizard.failed.llm")
              : t("plan.wizard.failed.heuristic")}
          </p>
          {detail && <p className="text-small text-ink-muted">{detail}</p>}
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button
            type="button"
            className="h-11 px-4 md:pointer-fine:h-9"
            disabled={pending}
            onClick={onRetry}
          >
            {t("common.retry")}
          </Button>
          {assistant && (
            <Button
              type="button"
              variant="outline"
              className="h-11 px-4 md:pointer-fine:h-9"
              disabled={pending}
              onClick={onWithoutAssistant}
            >
              {t("plan.wizard.failed.without_assistant")}
            </Button>
          )}
        </div>
      </div>
    )
  }

  const current = stepIndex(data?.stage)
  return (
    <div className="flex flex-col gap-3" data-testid="wizard-generating">
      <p role="status" className="sr-only">
        {t(`plan.progress.${STEPS[current]}`)}
      </p>
      <ol className="flex flex-col gap-3">
        {STEPS.map((step, i) => {
          const state =
            i < current ? "done" : i === current ? "current" : "pending"
          const Icon =
            state === "done"
              ? Check
              : state === "current"
                ? LoaderCircle
                : Circle
          return (
            <li
              key={step}
              aria-current={state === "current" ? "step" : undefined}
              data-state={state}
              className={cn(
                "flex items-center gap-3 text-body",
                state === "pending" ? "text-ink-muted" : "text-ink"
              )}
            >
              <Icon
                aria-hidden
                strokeWidth={1.5}
                className={cn(
                  "size-5 shrink-0",
                  state === "done" && "text-success",
                  state === "current" && "text-accent motion-safe:animate-spin",
                  state === "pending" && "size-4 text-line"
                )}
              />
              <span>{t(`plan.progress.${step}`)}</span>
              <span className="sr-only">
                {t(`plan.wizard.generating.${state}`)}
              </span>
            </li>
          )
        })}
      </ol>
    </div>
  )
}
