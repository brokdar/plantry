import { useTranslation } from "react-i18next"

import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"
import { cn } from "@/lib/utils"

interface MonthCellProps {
  date: string
  plates: Plate[]
  isCurrentMonth: boolean
  isToday: boolean
  matchesSearch: boolean | null
  onClick: (date: string) => void
  foodsById?: Map<number, Food>
}

const MAX_PREVIEWS = 3

export function MonthCell({
  date,
  plates,
  isCurrentMonth,
  isToday,
  matchesSearch,
  onClick,
  foodsById,
}: MonthCellProps) {
  const { t } = useTranslation()
  const day = parseInt(date.slice(8, 10), 10)
  const overflow =
    plates.length > MAX_PREVIEWS ? plates.length - MAX_PREVIEWS : 0
  const visible = plates.slice(0, MAX_PREVIEWS)

  const dimmed = matchesSearch === false

  return (
    <button
      type="button"
      onClick={() => onClick(date)}
      data-date={date}
      className={cn(
        "flex min-h-[80px] w-full flex-col gap-0.5 rounded-lg border p-1.5 text-left transition-[opacity,border-color,background-color] duration-150",
        isCurrentMonth
          ? "border-outline-variant/30 bg-surface-container-lowest hover:border-primary/40 hover:bg-surface-container-low"
          : "border-transparent bg-surface-container/30 text-on-surface-variant",
        isToday && "ring-2 ring-primary ring-offset-1",
        dimmed && "opacity-40"
      )}
    >
      {/* Date number */}
      <span
        className={cn(
          "font-heading text-xs leading-none font-bold",
          isToday
            ? "flex h-5 w-5 items-center justify-center rounded-full bg-primary text-[10px] text-on-primary"
            : isCurrentMonth
              ? "text-on-surface"
              : "text-on-surface-variant/60"
        )}
      >
        {day}
      </span>

      {/* Plate previews */}
      <div className="mt-0.5 flex flex-col gap-px">
        {visible.map((plate) => (
          <div
            key={plate.id}
            className={cn(
              "truncate rounded px-1 py-px text-[10px] leading-tight",
              plate.skipped
                ? "bg-outline-variant/20 text-on-surface-variant/60 line-through"
                : "bg-primary/10 text-primary"
            )}
          >
            {foodsById?.get(plate.components[0]?.food_id)?.name ??
              plate.note ??
              `${plate.components.length}×`}
          </div>
        ))}
        {overflow > 0 && (
          <div className="px-1 text-[10px] text-on-surface-variant">
            {t("calendar.overflow_more", { count: overflow })}
          </div>
        )}
      </div>
    </button>
  )
}
