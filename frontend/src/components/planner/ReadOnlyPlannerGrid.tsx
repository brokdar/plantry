import * as Lucide from "lucide-react"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import type { Component } from "@/lib/api/components"
import type { TimeSlot } from "@/lib/api/slots"
import type { Week } from "@/lib/api/weeks"
import { useComponents } from "@/lib/queries/components"
import { findPlateAt } from "@/lib/queries/plate-patches"
import { slotLabel } from "@/lib/slot-label"

const DAY_KEYS = [
  "planner.day_mon",
  "planner.day_tue",
  "planner.day_wed",
  "planner.day_thu",
  "planner.day_fri",
  "planner.day_sat",
  "planner.day_sun",
] as const

interface ReadOnlyPlannerGridProps {
  week: Week
  slots: TimeSlot[]
}

function SlotIcon({ name }: { name: string }) {
  const Icon = (
    Lucide as unknown as Record<string, Lucide.LucideIcon | undefined>
  )[name]
  if (!Icon) return <Lucide.HelpCircle className="h-4 w-4" aria-hidden />
  return <Icon className="h-4 w-4" aria-hidden />
}

export function ReadOnlyPlannerGrid({ week, slots }: ReadOnlyPlannerGridProps) {
  const { t } = useTranslation()
  const componentsQuery = useComponents({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Component>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  return (
    <div className="overflow-x-auto">
      <div
        className="grid min-w-[900px] gap-1"
        style={{ gridTemplateColumns: `120px repeat(7, minmax(0, 1fr))` }}
      >
        <div />
        {DAY_KEYS.map((dayKey, idx) => (
          <div
            key={dayKey}
            className="text-center text-xs font-semibold tracking-wide text-muted-foreground uppercase"
            data-testid={`archive-day-header-${idx}`}
          >
            {t(dayKey)}
          </div>
        ))}

        {slots.map((slot) => (
          <div key={slot.id} className="contents">
            <div
              className="flex items-center gap-2 px-2 py-3 text-sm font-medium"
              data-testid={`archive-slot-row-${slot.id}`}
            >
              <SlotIcon name={slot.icon} />
              <span>{slotLabel(t, slot.name_key)}</span>
            </div>
            {DAY_KEYS.map((_, day) => {
              const plate = findPlateAt(week, day, slot.id)
              return (
                <div
                  key={`${slot.id}-${day}`}
                  data-testid={`archive-cell-${day}-${slot.id}`}
                  className="min-h-20"
                >
                  {plate ? (
                    <div className="flex min-h-20 min-w-0 flex-col gap-1 overflow-hidden rounded-md border border-border bg-card p-2">
                      {plate.components.map((pc) => {
                        const c = componentsById.get(pc.component_id)
                        return (
                          <div
                            key={pc.id}
                            className="flex items-center gap-2 rounded-md border border-border bg-card px-2 py-1 text-sm"
                            data-testid={`archive-plate-component-${pc.id}`}
                          >
                            <span className="flex-1 truncate">
                              {c?.name ?? `#${pc.component_id}`}
                            </span>
                            {pc.portions !== 1 && (
                              <Badge variant="secondary" className="text-xs">
                                ×{pc.portions}
                              </Badge>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  ) : (
                    <div className="min-h-20 rounded-md border border-dashed border-border/30 bg-card/10" />
                  )}
                </div>
              )
            })}
          </div>
        ))}
      </div>
    </div>
  )
}
