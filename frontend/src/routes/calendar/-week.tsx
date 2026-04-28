import { useTranslation } from "react-i18next"

import type { PlannerDay } from "@/components/planner/PlannerGrid"
import { PlannerGrid } from "@/components/planner/PlannerGrid"
import { ReadOnlyPlannerGrid } from "@/components/planner/ReadOnlyPlannerGrid"
import { Button } from "@/components/ui/button"
import type { TimeSlot } from "@/lib/api/slots"

interface WeekViewProps {
  days: PlannerDay[]
  slots: TimeSlot[]
  edit: boolean
  rangeFrom: string
  rangeTo: string
  onEditToggle: () => void
}

export function WeekView({
  days,
  slots,
  edit,
  rangeFrom,
  rangeTo,
  onEditToggle,
}: WeekViewProps) {
  const { t } = useTranslation()

  return (
    <div className="flex flex-col gap-4">
      <div className="flex justify-end">
        <Button
          variant={edit ? "default" : "outline"}
          size="sm"
          onClick={onEditToggle}
          className="font-heading text-xs tracking-wide"
        >
          {edit ? t("common.done") : t("common.edit")}
        </Button>
      </div>
      {edit ? (
        <PlannerGrid
          days={days}
          slots={slots}
          rangeFrom={rangeFrom}
          rangeTo={rangeTo}
        />
      ) : (
        <ReadOnlyPlannerGrid days={days} slots={slots} />
      )}
    </div>
  )
}
