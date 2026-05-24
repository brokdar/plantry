import { format, parseISO } from "date-fns"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import type { Food } from "@/lib/api/foods"
import type { TimeSlot } from "@/lib/api/slots"
import { useFoods } from "@/lib/queries/foods"
import { SLOT_ICONS, SLOT_ICON_FALLBACK } from "@/lib/slot-icons"
import { slotLabel } from "@/lib/slot-label"

import type { PlannerDay } from "./PlannerGrid"
import { SlotHero, type SlotHeroComponent } from "./SlotHero"
import { SlotSides, type SlotSideItem } from "./SlotSides"

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
  days: PlannerDay[]
  slots: TimeSlot[]
}

function SlotIcon({ name }: { name: string }) {
  const Icon = SLOT_ICONS[name] ?? SLOT_ICON_FALLBACK
  return <Icon className="h-4 w-4" aria-hidden />
}

export function ReadOnlyPlannerGrid({ days, slots }: ReadOnlyPlannerGridProps) {
  const { t } = useTranslation()
  const componentsQuery = useFoods({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Food>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  return (
    <div className="hide-scrollbar overflow-x-auto">
      <div className="editorial-shadow min-w-[960px] rounded-3xl border border-outline-variant/40 bg-surface-container-lowest p-5">
        <div
          className="grid gap-2.5"
          style={{
            gridTemplateColumns: `130px repeat(${days.length}, minmax(0, 1fr))`,
          }}
        >
          <div />
          {days.map((day, idx) => {
            const date = parseISO(day.date)
            const dayKey = DAY_KEYS[day.weekday] ?? DAY_KEYS[idx % 7]
            return (
              <div
                key={day.date}
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
              {days.map((day, dayIdx) => {
                const plate = day.plates.find((p) => p.slot_id === slot.id)
                return (
                  <div
                    key={`${slot.id}-${day.date}`}
                    data-testid={`archive-cell-${dayIdx}-${slot.id}`}
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

import type { Plate } from "@/lib/api/plates"

interface ReadOnlySlotProps {
  plate: Plate | undefined
  componentsById: Map<number, Food>
}

function ReadOnlySlot({ plate, componentsById }: ReadOnlySlotProps) {
  const { t } = useTranslation()
  if (!plate) {
    return (
      <div className="h-[178px] rounded-[14px] border border-dashed border-outline-variant/40 bg-surface-container-low/30" />
    )
  }
  if (plate.skipped) {
    return (
      <div
        className="flex h-[178px] flex-col items-center justify-center gap-1.5 rounded-[14px] border border-tertiary/25 text-center"
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
  if (sorted.length === 0) {
    return (
      <div className="h-[178px] rounded-[14px] border border-dashed border-outline-variant/40 bg-surface-container-low/30" />
    )
  }
  const componentMeta = sorted.map((pc) => {
    const c = componentsById.get(pc.food_id)
    const role = c?.kind === "composed" ? (c.role ?? null) : null
    return {
      name: c?.name ?? `#${pc.food_id}`,
      imagePath: c?.image_path ?? null,
      role,
    }
  })
  const hero = sorted[0]!
  const heroComponents: SlotHeroComponent[] = componentMeta
  const sideItems: SlotSideItem[] = componentMeta.slice(1)
  const heroRoleKey = componentMeta[0]!.role
  const roleLabel = heroRoleKey
    ? t(`planner.slot.role.${heroRoleKey}`, { defaultValue: heroRoleKey })
    : null
  return (
    <div className="flex h-[178px] flex-col overflow-hidden rounded-[14px] border border-outline-variant/50 bg-surface-container-lowest">
      <SlotHero components={heroComponents} heroRoleLabel={roleLabel} />
      <div className="flex min-h-0 flex-1 flex-col gap-1.5 px-2.5 py-2">
        <p
          className="truncate font-heading text-[13.5px] leading-tight font-bold tracking-tight"
          data-testid={`archive-plate-component-${hero.id}`}
        >
          {componentMeta[0]!.name}
        </p>
        <SlotSides items={sideItems} max={3} />
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
