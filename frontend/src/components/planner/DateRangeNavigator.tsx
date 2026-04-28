import { ChevronLeft, ChevronRight } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"

interface DateRangeNavigatorProps {
  from: string // YYYY-MM-DD, start of current window
  to: string // YYYY-MM-DD, end of current window
  days: number // window size (7 for now)
  planAnchor: string // "today" | "next_shopping_day" | "fixed_weekday"
  shoppingDay: number // 0=Monday…6=Sunday (backend convention)
  onPrev: () => void
  onNext: () => void
  onToday: () => void
  /** Navigate to the window that starts at actual today, regardless of anchor mode. */
  onJumpToToday: () => void
}

/**
 * Convert backend weekday (0=Monday…6=Sunday) to JS Date.getDay() (0=Sunday…6=Saturday).
 */
function toJsDay(backendDay: number): number {
  return (backendDay + 1) % 7
}

function formatRangeLabel(from: string, to: string, locale: string): string {
  const fromDate = new Date(from + "T00:00:00")
  const toDate = new Date(to + "T00:00:00")

  const sameYear = fromDate.getFullYear() === toDate.getFullYear()
  const currentYear = new Date().getFullYear()

  const fromFmt = new Intl.DateTimeFormat(locale, {
    month: "short",
    day: "numeric",
    year:
      !sameYear || fromDate.getFullYear() !== currentYear
        ? "numeric"
        : undefined,
  })
  const toFmt = new Intl.DateTimeFormat(locale, {
    month: "short",
    day: "numeric",
    year:
      !sameYear || toDate.getFullYear() !== currentYear ? "numeric" : undefined,
  })

  return `${fromFmt.format(fromDate)} – ${toFmt.format(toDate)}`
}

function formatWeekdayName(jsDay: number, locale: string): string {
  // Use a known reference date: 2023-01-01 is a Sunday (js day 0).
  // We need a date whose getDay() === jsDay.
  // Sunday 2023-01-01 → jsDay 0; Monday → jsDay 1; etc.
  const refSunday = new Date(2023, 0, 1) // Jan 1 2023 = Sunday
  const date = new Date(refSunday)
  date.setDate(refSunday.getDate() + jsDay)
  return new Intl.DateTimeFormat(locale, { weekday: "long" }).format(date)
}

export function DateRangeNavigator({
  from,
  to,
  days,
  planAnchor,
  shoppingDay,
  onPrev,
  onNext,
  onToday,
  onJumpToToday,
}: DateRangeNavigatorProps) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language ?? "en"

  const rangeLabel = formatRangeLabel(from, to, locale)
  const shoppingWeekdayName = formatWeekdayName(toJsDay(shoppingDay), locale)

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={onPrev}
          aria-label={t("planner.prev_window")}
        >
          <ChevronLeft className="h-4 w-4" />
          {t("planner.prev_window")}
        </Button>

        <div className="min-w-48 text-center text-sm font-medium">
          {rangeLabel}
        </div>

        <Button variant="outline" size="sm" onClick={onToday}>
          {t("planner.today_button")}
        </Button>

        <Button
          variant="outline"
          size="sm"
          onClick={onNext}
          aria-label={t("planner.next_window")}
        >
          {t("planner.next_window")}
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onJumpToToday}
          className="inline-flex items-center rounded-full border border-border bg-muted px-3 py-0.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        >
          {t("planner.anchor_today")}
        </button>

        {planAnchor === "next_shopping_day" && (
          <button
            type="button"
            onClick={onToday}
            className="inline-flex items-center rounded-full border border-border bg-muted px-3 py-0.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
          >
            {t("planner.anchor_shopping_day", { weekday: shoppingWeekdayName })}
          </button>
        )}
      </div>

      {/* Hidden from render but keeps days in scope to satisfy linter when days > 7 is wired later */}
      <span className="sr-only" aria-hidden="true" data-window-days={days} />
    </div>
  )
}
