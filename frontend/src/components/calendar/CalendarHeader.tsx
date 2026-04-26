import { ChevronLeft, ChevronRight } from "lucide-react"
import { useDeferredValue } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

import { JumpToDate } from "./JumpToDate"

export type CalendarMode = "month" | "week" | "agenda"

interface CalendarHeaderProps {
  mode: CalendarMode
  label: string
  onPrev: () => void
  onNext: () => void
  onToday: () => void
  onModeChange: (mode: CalendarMode) => void
  search: string
  onSearchChange: (s: string) => void
  onJumpToDate: (date: string) => void
  jumpValue?: string
  className?: string
}

const MODES: { value: CalendarMode; key: string }[] = [
  { value: "month", key: "calendar.mode_month" },
  { value: "week", key: "calendar.mode_week" },
  { value: "agenda", key: "calendar.mode_agenda" },
]

export function CalendarHeader({
  mode,
  label,
  onPrev,
  onNext,
  onToday,
  onModeChange,
  search,
  onSearchChange,
  onJumpToDate,
  jumpValue = "",
  className,
}: CalendarHeaderProps) {
  const { t } = useTranslation()
  const deferredSearch = useDeferredValue(search)

  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-2 border-b border-outline-variant/40 bg-surface-container-lowest px-4 py-3",
        className
      )}
      data-deferred-search={deferredSearch}
    >
      {/* Nav controls */}
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          onClick={onPrev}
          aria-label={t("common.previous")}
          className="h-8 w-8 text-on-surface-variant hover:text-on-surface"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={onToday}
          className="font-heading text-xs tracking-wide text-on-surface-variant hover:text-on-surface"
        >
          {t("planner.today")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={onNext}
          aria-label={t("common.next")}
          className="h-8 w-8 text-on-surface-variant hover:text-on-surface"
        >
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>

      {/* Month/period label */}
      <h2 className="min-w-[120px] font-heading text-base font-semibold text-on-surface">
        {label}
      </h2>

      {/* Jump to date */}
      <JumpToDate value={jumpValue} onSelect={onJumpToDate} />

      <div className="flex-1" />

      {/* Search */}
      <Input
        type="search"
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
        placeholder={t("calendar.search_placeholder")}
        className="h-8 w-48 border-outline-variant/60 bg-surface-container-lowest text-sm"
      />

      {/* Mode toggle */}
      <div className="flex items-center overflow-hidden rounded-md border border-outline-variant/60">
        {MODES.map(({ value, key }) => (
          <button
            key={value}
            type="button"
            onClick={() => onModeChange(value)}
            className={cn(
              "px-3 py-1.5 font-heading text-xs tracking-wide transition-colors",
              mode === value
                ? "bg-primary text-on-primary"
                : "bg-surface-container-lowest text-on-surface-variant hover:bg-surface-container-low hover:text-on-surface"
            )}
          >
            {t(key)}
          </button>
        ))}
      </div>
    </div>
  )
}
