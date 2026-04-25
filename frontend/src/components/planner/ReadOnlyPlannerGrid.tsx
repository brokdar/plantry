import {
  addDays,
  format,
  setISOWeek,
  setISOWeekYear,
  startOfISOWeek,
} from "date-fns"
import * as Lucide from "lucide-react"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { Badge } from "@/components/ui/badge"
import type { Food } from "@/lib/api/foods"
import type { TimeSlot } from "@/lib/api/slots"
import type { Week } from "@/lib/api/weeks"
import { imageURL } from "@/lib/image-url"
import { useFoods } from "@/lib/queries/foods"
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
  const componentsQuery = useFoods({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Food>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  const weekStart = useMemo(() => {
    const d = setISOWeekYear(new Date(), week.year)
    return startOfISOWeek(setISOWeek(d, week.week_number))
  }, [week.year, week.week_number])

  return (
    <div className="hide-scrollbar overflow-x-auto">
      <div className="editorial-shadow min-w-[960px] rounded-3xl border border-outline-variant/40 bg-surface-container-lowest p-5">
        <div
          className="grid gap-2.5"
          style={{ gridTemplateColumns: "130px repeat(7, minmax(0, 1fr))" }}
        >
          <div />
          {DAY_KEYS.map((dayKey, idx) => {
            const date = addDays(weekStart, idx)
            return (
              <div
                key={dayKey}
                className="flex flex-col items-start gap-1 border-b border-outline-variant/50 px-2.5 py-3 pb-3.5"
                data-testid={`archive-day-header-${idx}`}
              >
                <span className="font-heading text-[13px] font-bold tracking-widest text-on-surface uppercase">
                  {t(dayKey)}
                </span>
                <span className="text-[12px] text-on-surface-variant tabular-nums">
                  {format(date, "MMM d")}
                </span>
              </div>
            )
          })}

          {slots.map((slot) => (
            <div key={slot.id} className="contents">
              <div
                className="flex flex-col items-start justify-center gap-1.5 px-3"
                data-testid={`archive-slot-row-${slot.id}`}
              >
                <span className="grid size-6 place-items-center rounded-lg bg-surface-container text-on-surface-variant">
                  <SlotIcon name={slot.icon} />
                </span>
                <span className="font-heading text-[12.5px] font-bold tracking-[0.04em] text-on-surface uppercase">
                  {slotLabel(t, slot.name_key)}
                </span>
              </div>
              {DAY_KEYS.map((_, day) => {
                const plate = findPlateAt(week, day, slot.id)
                return (
                  <div
                    key={`${slot.id}-${day}`}
                    data-testid={`archive-cell-${day}-${slot.id}`}
                    className="h-[178px]"
                  >
                    <ReadOnlySlot
                      plate={plate}
                      componentsById={componentsById}
                    />
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

interface ReadOnlySlotProps {
  plate: ReturnType<typeof findPlateAt>
  componentsById: Map<number, Food>
}

function ReadOnlySlot({ plate, componentsById }: ReadOnlySlotProps) {
  const { t } = useTranslation()
  if (!plate) {
    return (
      <div className="h-full rounded-[14px] border border-dashed border-outline-variant/40 bg-surface-container-low/30" />
    )
  }
  if (plate.skipped) {
    return (
      <div
        className="flex h-full flex-col items-center justify-center gap-1.5 rounded-[14px] border border-tertiary/25 text-center"
        style={{
          backgroundImage:
            "repeating-linear-gradient(45deg, transparent 0 7px, rgba(75,96,120,0.16) 7px 8px), var(--surface-container-low)",
        }}
      >
        <span className="font-heading text-[10px] font-bold tracking-[0.18em] text-tertiary uppercase">
          {t("skip.label")}
        </span>
        {plate.note && (
          <span className="max-w-full truncate px-2 text-[11px] text-on-tertiary-fixed-variant italic">
            {plate.note}
          </span>
        )}
      </div>
    )
  }
  const sorted = [...plate.components].sort(
    (a, b) => a.sort_order - b.sort_order
  )
  const hero = sorted[0]
  const heroComp = hero ? componentsById.get(hero.food_id) : undefined
  const sides = sorted.slice(1).map((pc) => {
    const c = componentsById.get(pc.food_id)
    return c?.name ?? `#${pc.food_id}`
  })
  const heroRole =
    (heroComp?.kind === "composed" ? heroComp.role : null) ?? "main"
  const roleLabel = t(`planner.slot.role.${heroRole}`, {
    defaultValue: heroRole,
  })
  return (
    <div className="flex h-full flex-col overflow-hidden rounded-[14px] border border-outline-variant/50 bg-surface-container-lowest">
      <div className="relative h-24 overflow-hidden">
        {heroComp?.image_path ? (
          <img
            src={imageURL(heroComp.image_path)}
            alt=""
            className="h-full w-full object-cover"
          />
        ) : (
          <FoodPlaceholder
            category={heroRole as FoodPlaceholderCategory}
            size="md"
            rounded="none"
            className="h-full w-full"
          />
        )}
        {heroComp?.image_path && (
          <div
            className="absolute inset-0 bg-gradient-to-b from-transparent from-40% to-black/40"
            aria-hidden
          />
        )}
        <span className="absolute bottom-2 left-2 font-heading text-[9.5px] font-bold tracking-[0.16em] text-white uppercase drop-shadow-[0_1px_2px_rgba(0,0,0,0.45)]">
          {roleLabel}
        </span>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-1 px-2.5 py-2">
        <p className="truncate font-heading text-[13.5px] leading-tight font-bold tracking-tight">
          {heroComp?.name ?? (hero ? `#${hero.food_id}` : "")}
        </p>
        {sides.length > 0 && (
          <div className="flex gap-1 overflow-hidden">
            {sides.slice(0, 2).map((name, i) => (
              <span
                key={`${name}-${i}`}
                className="inline-flex min-w-0 items-center rounded-[5px] bg-surface-container-low px-1.5 py-0.5 text-[10.5px] text-on-surface-variant"
                data-testid={
                  hero ? `archive-plate-component-${hero.id}` : undefined
                }
              >
                <span className="truncate">{name}</span>
              </span>
            ))}
            {sides.length > 2 && (
              <span className="inline-flex items-center rounded-[5px] border border-dashed border-outline-variant px-1.5 py-0.5 text-[10.5px] font-semibold text-on-surface-variant">
                {t("planner.slot.overflow", { count: sides.length - 2 })}
              </span>
            )}
          </div>
        )}
        {plate.components
          .filter((pc) => pc.portions !== 1)
          .slice(0, 1)
          .map((pc) => (
            <Badge
              key={pc.id}
              variant="secondary"
              className="h-4 w-fit px-1.5 text-[10px]"
              data-testid={`archive-plate-component-${pc.id}`}
            >
              ×{pc.portions}
            </Badge>
          ))}
      </div>
    </div>
  )
}
