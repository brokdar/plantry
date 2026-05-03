import { ChevronLeft, ChevronRight } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"

interface DateRangeNavigatorProps {
  from: string // YYYY-MM-DD, start of current window
  to: string // YYYY-MM-DD, end of current window
  onPrev: () => void
  onNext: () => void
  onToday: () => void
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

export function DateRangeNavigator({
  from,
  to,
  onPrev,
  onNext,
  onToday,
}: DateRangeNavigatorProps) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language ?? "en"

  const rangeLabel = formatRangeLabel(from, to, locale)

  return (
    <div className="flex items-center gap-1.5">
      <Button
        variant="ghost"
        size="icon"
        onClick={onPrev}
        aria-label={t("planner.prev_window")}
        className="size-9 hover:bg-primary/10 hover:text-primary"
      >
        <ChevronLeft className="size-4" />
      </Button>

      <div className="min-w-32 px-2 text-center font-heading text-sm font-bold tracking-tight text-on-surface tabular-nums">
        {rangeLabel}
      </div>

      <Button
        variant="ghost"
        size="icon"
        onClick={onNext}
        aria-label={t("planner.next_window")}
        className="size-9 hover:bg-primary/10 hover:text-primary"
      >
        <ChevronRight className="size-4" />
      </Button>

      <Button
        variant="ghost"
        size="sm"
        onClick={onToday}
        className="ml-1 h-9 px-3 text-xs font-bold tracking-widest text-on-surface-variant uppercase hover:bg-primary/10 hover:text-primary"
      >
        {t("planner.today_button")}
      </Button>
    </div>
  )
}
