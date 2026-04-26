import { getISOWeek, getISOWeekYear, startOfWeek } from "date-fns"
import { useTranslation } from "react-i18next"

import type { Plate } from "@/lib/api/plates"

import { AgendaGroup } from "./AgendaGroup"

interface AgendaListProps {
  plates: Plate[]
  hasNextPage: boolean
  isFetchingNextPage: boolean
  fetchNextPage: () => void
  search: string
  weekStartsOn: 0 | 1 | 6
  foodsById?: Map<number, string>
  showCopyButton?: boolean
}

interface WeekBucket {
  key: string
  weekLabel: string
  plates: Plate[]
}

function groupByWeek(plates: Plate[], weekStartsOn: 0 | 1 | 6): WeekBucket[] {
  const buckets = new Map<string, WeekBucket>()

  for (const plate of plates) {
    const date = new Date(plate.date)
    const weekStart = startOfWeek(date, { weekStartsOn })
    const isoWeek = getISOWeek(weekStart)
    const isoYear = getISOWeekYear(weekStart)
    const key = `${isoYear}-W${String(isoWeek).padStart(2, "0")}`

    if (!buckets.has(key)) {
      buckets.set(key, {
        key,
        weekLabel: `${isoYear} W${isoWeek}`,
        plates: [],
      })
    }
    buckets.get(key)!.plates.push(plate)
  }

  // Sort buckets newest first
  return [...buckets.values()].sort((a, b) => b.key.localeCompare(a.key))
}

export function AgendaList({
  plates,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  search,
  weekStartsOn,
  foodsById,
  showCopyButton,
}: AgendaListProps) {
  const { t } = useTranslation()

  const filtered =
    search.trim() === ""
      ? plates
      : plates.filter((plate) =>
          plate.components.some((c) => {
            const name = foodsById?.get(c.food_id) ?? ""
            return name.toLowerCase().includes(search.toLowerCase())
          })
        )

  const buckets = groupByWeek(filtered, weekStartsOn)

  if (plates.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-on-surface-variant">
        {t("calendar.no_plates_in_range")}
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      {buckets.map((bucket, i) => (
        <AgendaGroup
          key={bucket.key}
          weekLabel={bucket.weekLabel}
          plates={bucket.plates}
          defaultOpen={i === 0}
          foodsById={foodsById}
          showCopyButton={showCopyButton}
        />
      ))}

      {hasNextPage && (
        <div className="flex justify-center pt-2">
          <button
            type="button"
            onClick={fetchNextPage}
            disabled={isFetchingNextPage}
            className="rounded-lg px-4 py-2 text-sm font-medium text-primary hover:bg-primary/10 disabled:opacity-50"
          >
            {t("calendar.load_older")}
          </button>
        </div>
      )}
    </div>
  )
}
