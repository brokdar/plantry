import { useEffect, useEffectEvent } from "react"
import { useTranslation } from "react-i18next"

import type { PlanRun, RunStage } from "@/lib/api/generation"
import { toast } from "@/lib/toast"

/** The stages in order (D35); the bar fills one step per stage. */
const STAGES: RunStage[] = [
  "resolving",
  "choosing",
  "checking",
  "repairing",
  "saving",
]

interface GenerationProgressProps {
  /** The run the board follows; undefined until its first poll returns. */
  run: PlanRun | undefined
  /** Called once the run has finished and its result was announced. */
  onFinished: () => void
}

/**
 * The board header's progress bar while a generation runs (D35, UX §5.5):
 * the board stays visible, the bar names the current stage. When the run
 * finishes it announces the result in a toast ("Planned 10 plates, 4 cook
 * days.", "Plan regenerated. 2 confirmed plates kept.", or the error) and
 * calls `onFinished`.
 */
export function GenerationProgress({
  run,
  onFinished,
}: GenerationProgressProps) {
  const { t } = useTranslation()

  const finish = useEffectEvent((done: PlanRun) => {
    if (done.status === "error") {
      toast.error(
        t(done.error_key ?? "plan.run.failed", {
          defaultValue: t("plan.run.failed"),
        })
      )
    } else {
      const { counts } = done
      const parts = [
        counts.kept_confirmed > 0
          ? t("plan.run.regenerated_kept", { count: counts.kept_confirmed })
          : done.kind === "regenerate"
            ? t("plan.run.regenerated")
            : t("plan.run.planned", {
                plates: t("plan.toolbar.plates", { count: counts.plates }),
                cook_days: t("plan.toolbar.cook_days", {
                  count: counts.cook_days,
                }),
              }),
      ]
      const attention = done.issues.filter(
        (i) => i.severity === "violation"
      ).length
      if (attention > 0) {
        parts.push(`${t("plan.toolbar.warnings", { count: attention })}.`)
      }
      toast(parts.join(" "))
    }
    onFinished()
  })

  useEffect(() => {
    if (run && run.status !== "running") finish(run)
  }, [run])

  if (run && run.status !== "running") return null

  const stage = run?.stage ?? null
  const step = stage ? STAGES.indexOf(stage) + 1 : 0
  const label = stage ? t(`plan.progress.${stage}`) : t("plan.progress.label")

  return (
    <div className="flex flex-col gap-1.5" data-testid="generation-progress">
      <p role="status" className="text-small text-ink">
        {label}
      </p>
      <div
        role="progressbar"
        aria-label={t("plan.progress.label")}
        aria-valuemin={0}
        aria-valuemax={STAGES.length}
        aria-valuenow={step}
        aria-valuetext={label}
        className="h-1 w-full max-w-md overflow-hidden rounded-full bg-panel-sunken"
      >
        <div
          className="h-full rounded-full bg-accent motion-safe:transition-[width] motion-safe:duration-300"
          style={{ width: `${(Math.max(step, 0.5) / STAGES.length) * 100}%` }}
        />
      </div>
    </div>
  )
}
