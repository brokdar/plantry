import { Link } from "@tanstack/react-router"
import { CalendarRange, Sparkles } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"

interface EmptyWindowProps {
  /** The window's start: "Plan this week" opens the wizard for it. */
  start: string
  /** Focuses the first open cell of the board. */
  onPlanByHand: () => void
  onCopyWeek: () => void
}

/**
 * The teaching state of a window without plates (UX §5.5, §6): "Plan this
 * week" opens the plan wizard (below the recipe target its route sends the
 * user to Recipes); copying a week or planning by hand are the quieter
 * ways. Shown above the empty board so "or plan by hand" lands in it.
 */
export function EmptyWindow({
  start,
  onPlanByHand,
  onCopyWeek,
}: EmptyWindowProps) {
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
      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        <Button asChild className="h-11 gap-2 px-4">
          <Link
            to="/plan/new"
            search={{ start }}
            data-testid="plan-empty-wizard"
          >
            <Sparkles strokeWidth={1.5} aria-hidden="true" />
            {t("plan.empty.plan_week")}
          </Link>
        </Button>
        <Button
          variant="outline"
          onClick={onCopyWeek}
          className="h-11 gap-2 px-4"
        >
          <CalendarRange strokeWidth={1.5} aria-hidden="true" />
          {t("plan.empty.copy")}
        </Button>
        <Button
          variant="link"
          onClick={onPlanByHand}
          className="h-11 px-2 text-accent"
        >
          {t("plan.empty.by_hand")}
        </Button>
      </div>
    </section>
  )
}
