import { format, isToday, parseISO } from "date-fns"
import * as Lucide from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import type { Food } from "@/lib/api/foods"
import type { TimeSlot } from "@/lib/api/slots"
import { addPlateComponent, createPlate } from "@/lib/api/plates"
import { useFoods, useSetFoodFavorite } from "@/lib/queries/foods"
import { useSetPlateSkipped } from "@/lib/queries/plates"
import { queryClient } from "@/lib/query-client"
import { plateKeys } from "@/lib/queries/keys"
import { slotLabel } from "@/lib/slot-label"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toastError } from "@/lib/toast"
import { cn } from "@/lib/utils"

import { AddComponentSheet } from "./AddComponentSheet"
import type { PlannerDay } from "./PlannerGrid"
import { SlotCell } from "./SlotCell"

const DAY_KEYS = [
  "planner.day_mon",
  "planner.day_tue",
  "planner.day_wed",
  "planner.day_thu",
  "planner.day_fri",
  "planner.day_sat",
  "planner.day_sun",
] as const

interface MobilePlannerGridProps {
  days: PlannerDay[]
  slots: TimeSlot[]
  rangeFrom: string
  rangeTo: string
}

function SlotIcon({ name }: { name: string }) {
  const Icon = (
    Lucide as unknown as Record<string, Lucide.LucideIcon | undefined>
  )[name]
  if (!Icon) return <Lucide.HelpCircle className="h-4 w-4" aria-hidden />
  return <Icon className="h-4 w-4" aria-hidden />
}

interface AddTarget {
  dayIdx: number
  slotId: number
}

export function MobilePlannerGrid({
  days,
  slots,
  rangeFrom,
  rangeTo,
}: MobilePlannerGridProps) {
  const { t } = useTranslation()

  const componentsQuery = useFoods({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Food>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  // Default to today's index; fall back to 0 if today isn't in the window.
  const todayStr = new Date().toISOString().slice(0, 10)
  const todayIdx = days.findIndex((d) => d.date === todayStr)
  const [activeDay, setActiveDay] = useState(todayIdx >= 0 ? todayIdx : 0)
  const [addTarget, setAddTarget] = useState<AddTarget | null>(null)

  const setFavoriteMut = useSetFoodFavorite()
  const setSkippedMut = useSetPlateSkipped()
  const clearAiFillOnPlate = usePlannerUI((s) => s.clearAiFillOnPlate)

  const openPicker = (dayIdx: number, slotId: number) => {
    setAddTarget({ dayIdx, slotId })
  }

  async function handlePick(component: Food) {
    if (!addTarget) return
    const target = addTarget
    setAddTarget(null)
    const targetDay = days[target.dayIdx]
    if (!targetDay) return
    try {
      const created = await createPlate({
        date: targetDay.date,
        slot_id: target.slotId,
      })
      await addPlateComponent(created.id, {
        food_id: component.id,
        portions: 1,
      })
      void queryClient.invalidateQueries({
        queryKey: plateKeys.range(rangeFrom, rangeTo),
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleToggleSkip(
    dayIdx: number,
    slotId: number,
    plateId: number | null
  ) {
    const targetDay = days[dayIdx]
    if (!targetDay) return
    try {
      const existing = targetDay.plates.find((p) => p.slot_id === slotId)
      if (plateId === null) return // no plate to skip in mobile view
      await setSkippedMut.mutateAsync({
        plateId,
        input: { skipped: !existing?.skipped, note: existing?.note ?? null },
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleToggleFavorite(
    componentId: number | undefined,
    current: boolean
  ) {
    if (!componentId) return
    try {
      await setFavoriteMut.mutateAsync({ id: componentId, favorite: !current })
    } catch (err) {
      toastError(err, t)
    }
  }

  const activeData = days[activeDay]

  return (
    <div className="flex flex-col gap-4">
      <div
        className="grid gap-1 rounded-2xl bg-surface-container-low p-2"
        style={{
          gridTemplateColumns: `repeat(${days.length}, minmax(0, 1fr))`,
        }}
        role="tablist"
        aria-label={t("planner.title")}
      >
        {days.map((day, idx) => {
          const date = parseISO(day.date)
          const active = idx === activeDay
          const dayIsToday = isToday(date)
          const dayKey = DAY_KEYS[day.weekday] ?? DAY_KEYS[idx % 7]
          return (
            <button
              key={day.date}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => setActiveDay(idx)}
              data-testid={`mobile-day-tab-${idx}`}
              className={cn(
                "flex flex-col items-center gap-0.5 rounded-xl py-2",
                active && "bg-surface-container-high"
              )}
            >
              <span
                className={cn(
                  "font-heading text-[10px] font-bold tracking-[0.1em] uppercase",
                  active ? "text-primary" : "text-on-surface-variant"
                )}
              >
                {t(dayKey)}
              </span>
              <span
                className={cn(
                  "font-heading text-[15px] font-bold",
                  dayIsToday && !active && "text-primary"
                )}
              >
                {format(date, "d")}
              </span>
            </button>
          )
        })}
      </div>

      <ul className="flex flex-col gap-3">
        {slots.map((slot) => {
          const plate = activeData?.plates.find((p) => p.slot_id === slot.id)
          return (
            <li
              key={slot.id}
              data-testid={`mobile-cell-${activeDay}-${slot.id}`}
            >
              <div className="mb-1.5 flex items-center gap-2 px-1">
                <SlotIcon name={slot.icon} />
                <span className="font-heading text-[11px] font-bold tracking-[0.16em] text-on-surface-variant uppercase">
                  {slotLabel(t, slot.name_key)}
                </span>
              </div>
              <SlotCell
                day={activeDay}
                slotId={slot.id}
                plate={plate}
                componentsById={componentsById}
                onAdd={() => openPicker(activeDay, slot.id)}
                onDeletePlate={() => {}}
                onToggleFavorite={() => {
                  const hero = plate?.components
                    .slice()
                    .sort((a, b) => a.sort_order - b.sort_order)[0]
                  const heroComp = hero
                    ? componentsById.get(hero.food_id)
                    : undefined
                  void handleToggleFavorite(
                    heroComp?.id,
                    heroComp?.favorite ?? false
                  )
                  if (plate) clearAiFillOnPlate(plate.id)
                }}
                onToggleSkip={() => {
                  void handleToggleSkip(activeDay, slot.id, plate?.id ?? null)
                  if (plate) clearAiFillOnPlate(plate.id)
                }}
                onRateLoved={() => {}}
                onRateDisliked={() => {}}
              />
            </li>
          )
        })}
      </ul>

      <AddComponentSheet
        open={addTarget !== null}
        onOpenChange={(o) => !o && setAddTarget(null)}
        onPick={handlePick}
      />
    </div>
  )
}
