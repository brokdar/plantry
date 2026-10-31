import { Link } from "@tanstack/react-router"
import { ChevronLeft, ChevronRight, Settings } from "lucide-react"
import { useTranslation } from "react-i18next"

import { shiftDate } from "@/lib/plan/today"
import { cn } from "@/lib/utils"

interface VerdictProps {
  /** The day shown (YYYY-MM-DD). */
  date: string
  /** The real today, for "Back to today". */
  today: string
  /** "Monday, 2 November". */
  dateLine: string
  /** Exactly one verdict, e.g. "Nothing to cook today". */
  headline: string
}

const ICON_LINK =
  "inline-flex size-11 items-center justify-center rounded-lg text-ink-muted transition-colors duration-150 ease-out hover:bg-panel-sunken hover:text-ink md:pointer-fine:size-9"

/**
 * Today's header (UX §5.2): the date line with day navigation, the phone
 * Settings gear (D31; the rail carries it from 768 px up), then the
 * Newsreader headline with the day's verdict.
 */
export function Verdict({ date, today, dateLine, headline }: VerdictProps) {
  const { t } = useTranslation()
  return (
    <header className="flex flex-col gap-1">
      <div className="flex items-center gap-1">
        <p
          className="min-w-0 flex-1 truncate text-small text-ink-muted"
          data-testid="today-date"
        >
          {dateLine}
        </p>
        {date !== today && (
          <Link
            to="/today"
            aria-label={t("today.back_to_today")}
            className="inline-flex min-h-11 items-center rounded-lg px-2 text-small text-accent hover:underline md:pointer-fine:min-h-9"
          >
            {t("nav.today")}
          </Link>
        )}
        <Link
          to="/today"
          search={{ date: shiftDate(date, -1) }}
          aria-label={t("today.prev_day")}
          className={ICON_LINK}
        >
          <ChevronLeft className="size-5" strokeWidth={1.5} aria-hidden />
        </Link>
        <Link
          to="/today"
          search={{ date: shiftDate(date, 1) }}
          aria-label={t("today.next_day")}
          className={ICON_LINK}
        >
          <ChevronRight className="size-5" strokeWidth={1.5} aria-hidden />
        </Link>
        <Link
          to="/settings"
          aria-label={t("nav.settings")}
          className={cn(ICON_LINK, "md:hidden")}
          data-testid="today-settings"
        >
          <Settings className="size-5" strokeWidth={1.5} aria-hidden />
        </Link>
      </div>
      <h1
        className="font-heading text-[1.625rem] leading-tight font-semibold text-ink md:text-h1"
        data-testid="today-verdict"
      >
        {headline}
      </h1>
    </header>
  )
}
