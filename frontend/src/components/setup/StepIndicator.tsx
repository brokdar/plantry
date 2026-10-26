import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

/** "Step 2 of 7" over a row of short bars, one per step. `step` is 1-based. */
export function StepIndicator({
  step,
  total,
}: {
  step: number
  total: number
}) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-2" data-testid="setup-step-indicator">
      <p className="text-small text-ink-muted tabular-nums">
        {t("setup.step_of", { step, total })}
      </p>
      <ol className="flex gap-1" aria-hidden>
        {Array.from({ length: total }, (_, i) => (
          <li
            key={i}
            className={cn(
              "h-1 flex-1 rounded-full transition-colors duration-200 ease-out",
              i < step ? "bg-accent" : "bg-line"
            )}
          />
        ))}
      </ol>
    </div>
  )
}
