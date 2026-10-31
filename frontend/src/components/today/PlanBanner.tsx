import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { shiftYMD } from "@/lib/dates"
import { formatDate } from "@/lib/plan/format"

/** "Plan next week" (UX §5.2): shown from the day before the next window
 *  starts until it has a plate (`plan_banner.due`). */
export function PlanBanner({
  nextStart,
  today,
}: {
  nextStart: string
  /** The real date, so a week starting today or tomorrow says so. */
  today: string
}) {
  const { t, i18n } = useTranslation()
  const body =
    nextStart === today
      ? t("today.banner.body_today")
      : nextStart === shiftYMD(today, 1)
        ? t("today.banner.body_tomorrow")
        : t("today.banner.body", {
            date: formatDate(nextStart, i18n, {
              weekday: "long",
              day: "numeric",
              month: "long",
            }),
          })
  return (
    <section
      aria-labelledby="today-banner"
      className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-line bg-panel p-4"
      data-testid="today-plan-banner"
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <h2
          id="today-banner"
          className="font-sans text-body font-semibold text-ink"
        >
          {t("today.banner.title")}
        </h2>
        <p className="text-small text-ink-muted">{body}</p>
      </div>
      <Button asChild className="h-11 px-4 md:pointer-fine:h-9">
        <Link to="/plan" search={{ start: nextStart }}>
          {t("today.banner.action")}
        </Link>
      </Button>
    </section>
  )
}
