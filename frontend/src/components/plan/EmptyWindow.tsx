import { CalendarRange, Hand } from "lucide-react"
import { useTranslation } from "react-i18next"

import { GeneratePlanButton } from "@/components/shell/GeneratePlanButton"
import { Button } from "@/components/ui/button"

interface EmptyWindowProps {
  /** Focuses the first open cell of the board. */
  onPlanByHand: () => void
  onCopyWeek: () => void
}

/**
 * The teaching state of a window without plates (UX §5.5, §6): three ways
 * to start, shown above the empty board so "Plan by hand" lands in it.
 */
export function EmptyWindow({ onPlanByHand, onCopyWeek }: EmptyWindowProps) {
  const { t } = useTranslation()
  return (
    <section
      aria-labelledby="plan-empty-title"
      data-testid="plan-empty"
      className="flex flex-col gap-4 rounded-lg border border-line bg-panel p-4 md:flex-row md:items-center md:justify-between md:p-6"
    >
      <div className="flex max-w-[52ch] flex-col gap-1">
        <h2 id="plan-empty-title" className="text-h3 font-semibold text-ink">
          {t("plan.empty.title")}
        </h2>
        <p className="text-body text-ink-muted">{t("plan.empty.body")}</p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
        <Button
          variant="outline"
          onClick={onPlanByHand}
          className="h-11 gap-2 px-4"
        >
          <Hand strokeWidth={1.5} aria-hidden="true" />
          {t("plan.empty.by_hand")}
        </Button>
        <Button
          variant="outline"
          onClick={onCopyWeek}
          className="h-11 gap-2 px-4"
        >
          <CalendarRange strokeWidth={1.5} aria-hidden="true" />
          {t("plan.empty.copy")}
        </Button>
        <div className="sm:w-48">
          <GeneratePlanButton />
        </div>
      </div>
    </section>
  )
}
