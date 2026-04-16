import { ChevronLeft, ChevronRight, Copy } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"

interface WeekNavigatorProps {
  year: number
  weekNumber: number
  onPrev: () => void
  onNext: () => void
  onCopy: () => void
}

export function WeekNavigator({
  year,
  weekNumber,
  onPrev,
  onNext,
  onCopy,
}: WeekNavigatorProps) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        size="icon"
        onClick={onPrev}
        aria-label={t("planner.prev_week")}
      >
        <ChevronLeft className="h-4 w-4" />
      </Button>
      <div className="min-w-32 text-center text-sm font-medium">
        {t("planner.week_label", { week: weekNumber, year })}
      </div>
      <Button
        variant="outline"
        size="icon"
        onClick={onNext}
        aria-label={t("planner.next_week")}
      >
        <ChevronRight className="h-4 w-4" />
      </Button>
      <Button variant="outline" size="sm" onClick={onCopy}>
        <Copy className="h-4 w-4" />
        {t("planner.copy_week")}
      </Button>
    </div>
  )
}
