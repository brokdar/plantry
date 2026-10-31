import { Copy, MoreHorizontal, Pencil, Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Food } from "@/lib/api/foods"
import type { Preset } from "@/lib/api/presets"
import type { TimeSlot } from "@/lib/api/slots"
import { slotLabel } from "@/lib/slot-label"

interface RoutineCardProps {
  routine: Preset
  foodsById: Map<number, Food>
  slotsById: Map<number, TimeSlot>
  /** For "Used 3 days ago"; passed in so render stays pure. */
  now: number
  onOpen: () => void
  onRename: () => void
  onDuplicate: () => void
  onDelete: () => void
}

const DAY = 24 * 60 * 60 * 1000

/** "3 days ago", "last month" in the UI language. */
function relativeWhen(iso: string, now: number, lang: string) {
  const time = new Date(iso).getTime()
  if (Number.isNaN(time)) return null
  const days = Math.round((time - now) / DAY)
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: "auto" })
  if (Math.abs(days) < 30) return rtf.format(days, "day")
  if (Math.abs(days) < 365) return rtf.format(Math.round(days / 30), "month")
  return rtf.format(Math.round(days / 365), "year")
}

/** One routine in Settings → Meals: its meals and foods at a glance; the
 *  name opens the editor, the menu renames, duplicates or deletes. */
export function RoutineCard({
  routine,
  foodsById,
  slotsById,
  now,
  onOpen,
  onRename,
  onDuplicate,
  onDelete,
}: RoutineCardProps) {
  const { t, i18n } = useTranslation()

  const meals = routine.plates
    .map((p) => slotsById.get(p.slot_id))
    .filter((s): s is TimeSlot => s !== undefined)
    .map((s) => slotLabel(t, s.name_key))
  const foods = [
    ...new Set(
      routine.plates.flatMap((p) =>
        p.components.map(
          (c) => foodsById.get(c.food_id)?.name ?? `#${c.food_id}`
        )
      )
    ),
  ]
  const when = routine.last_used_at
    ? relativeWhen(routine.last_used_at, now, i18n.language)
    : null

  return (
    <article
      className="flex h-full flex-col gap-2 rounded-cell border border-line bg-panel p-3"
      data-testid="routine-card"
    >
      <div className="flex items-start gap-2">
        <button
          type="button"
          onClick={onOpen}
          className="min-h-11 flex-1 rounded-cell text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent md:pointer-fine:min-h-0"
        >
          <h4 className="text-body font-semibold text-ink">{routine.name}</h4>
          <p className="text-small text-ink-muted">
            {meals.length > 0
              ? meals.join(" · ")
              : t("routines.meals_count", { count: routine.plates.length })}
          </p>
        </button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className="size-11 text-ink-muted md:pointer-fine:size-8"
              aria-label={t("routines.actions", { name: routine.name })}
            >
              <MoreHorizontal strokeWidth={1.5} aria-hidden />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={onOpen}>
              <Pencil strokeWidth={1.5} aria-hidden />
              {t("common.edit")}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={onRename}>
              <Pencil strokeWidth={1.5} aria-hidden />
              {t("routines.rename")}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={onDuplicate}>
              <Copy strokeWidth={1.5} aria-hidden />
              {t("routines.duplicate")}
            </DropdownMenuItem>
            <DropdownMenuItem variant="destructive" onSelect={onDelete}>
              <Trash2 strokeWidth={1.5} aria-hidden />
              {t("routines.delete")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {foods.length > 0 && (
        <p className="line-clamp-2 text-small text-ink">{foods.join(", ")}</p>
      )}

      <div className="mt-auto flex flex-wrap items-center gap-1.5 pt-1">
        {routine.tags.map((tag) => (
          <span
            key={tag}
            className="rounded-cell bg-panel-sunken px-1.5 py-0.5 text-caption text-ink-muted"
          >
            {tag}
          </span>
        ))}
        <span className="ml-auto text-caption text-ink-muted">
          {when
            ? t("routines.last_used", { when })
            : t("routines.last_used_never")}
        </span>
      </div>
    </article>
  )
}
