import {
  addDays,
  format,
  isSameDay,
  setISOWeek,
  setISOWeekYear,
  startOfISOWeek,
} from "date-fns"
import * as Lucide from "lucide-react"
import { useNavigate } from "@tanstack/react-router"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import type { Component } from "@/lib/api/components"
import type { TimeSlot } from "@/lib/api/slots"
import type { Week } from "@/lib/api/weeks"
import {
  useComponents,
  useSetComponentFavorite,
} from "@/lib/queries/components"
import { findPlateAt } from "@/lib/queries/plate-patches"
import { useSetPlateSkipped } from "@/lib/queries/plates"
import { useWeekNutrition, useCreatePlate } from "@/lib/queries/weeks"
import { slotLabel } from "@/lib/slot-label"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toastError } from "@/lib/toast"
import { cn } from "@/lib/utils"

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

export function MobilePlannerGrid({ week, slots }: MobilePlannerGridProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const componentsQuery = useComponents({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Component>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  const weekStart = useMemo(() => {
    const d = setISOWeekYear(new Date(), week.year)
    return startOfISOWeek(setISOWeek(d, week.week_number))
  }, [week.year, week.week_number])

  const today = new Date()
  const todayIdx = DAY_KEYS.findIndex((_, i) =>
    isSameDay(addDays(weekStart, i), today)
  )
  const [activeDay, setActiveDay] = useState(todayIdx >= 0 ? todayIdx : 0)

  const nutritionQuery = useWeekNutrition(week.id)
  const dayMacros = useMemo(() => {
    const m = new Map<
      number,
      NonNullable<typeof nutritionQuery.data>["days"][number]["macros"]
    >()
    for (const d of nutritionQuery.data?.days ?? []) m.set(d.day, d.macros)
    return m
  }, [nutritionQuery, nutritionQuery.data])

  const activeMacros = dayMacros.get(activeDay)

  const setFavoriteMut = useSetComponentFavorite()
  const setSkippedMut = useSetPlateSkipped(week.id)
  const createPlateMut = useCreatePlate(week.id)
  const clearAiFillOnPlate = usePlannerUI((s) => s.clearAiFillOnPlate)

  const openPicker = (day: number, slotId: number) =>
    void navigate({
      to: "/planner/$weekId/$day/$slotId/pick",
      params: {
        weekId: String(week.id),
        day: String(day),
        slotId: String(slotId),
      },
    })

  async function handleToggleSkip(
    day: number,
    slotId: number,
    plateId: number | null
  ) {
    try {
      let id = plateId
      if (id === null) {
        const created = await createPlateMut.mutateAsync({
          day,
          slot_id: slotId,
        })
        id = created.id
      }
      const existing = findPlateAt(week, day, slotId)
      await setSkippedMut.mutateAsync({
        plateId: id,
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

  return (
    <div className="flex flex-col gap-4">
      <div
        className="grid grid-cols-7 gap-1 rounded-2xl bg-surface-container-low p-2"
        role="tablist"
        aria-label={t("planner.title")}
      >
        {DAY_KEYS.map((dayKey, idx) => {
          const date = addDays(weekStart, idx)
          const active = idx === activeDay
          const isToday = isSameDay(date, today)
          return (
            <button
              key={dayKey}
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
                  isToday && !active && "text-primary"
                )}
              >
                {format(date, "d")}
              </span>
            </button>
          )
        })}
      </div>

      {activeMacros && (
        <div className="flex items-baseline justify-between px-1">
          <span className="font-heading text-[11px] font-bold tracking-[0.16em] text-on-surface-variant uppercase">
            {t(DAY_KEYS[activeDay])}
          </span>
          <span className="font-heading text-[18px] font-bold tracking-tight">
            {Math.round(activeMacros.kcal).toLocaleString()}
            <span className="ml-1 text-[11px] font-medium text-on-surface-variant">
              kcal
            </span>
          </span>
        </div>
      )}

      <ul className="flex flex-col gap-3">
        {slots.map((slot) => {
          const plate = findPlateAt(week, activeDay, slot.id)
          return (
            <li key={slot.id}>
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
                    ? componentsById.get(hero.component_id)
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
    </div>
  )
}
