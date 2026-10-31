import type { PlannerDay } from "@/components/calendar/ReadOnlyPlannerGrid"
import { ReadOnlyPlannerGrid } from "@/components/calendar/ReadOnlyPlannerGrid"
import type { TimeSlot } from "@/lib/api/slots"

interface WeekViewProps {
  days: PlannerDay[]
  slots: TimeSlot[]
}

export function WeekView({ days, slots }: WeekViewProps) {
  return <ReadOnlyPlannerGrid days={days} slots={slots} />
}
