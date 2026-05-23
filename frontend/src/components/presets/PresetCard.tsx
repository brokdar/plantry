import { Copy, MoreVertical, Pencil, Trash2 } from "lucide-react"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
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

interface PresetCardProps {
  preset: Preset
  foodsById: Map<number, Food>
  slotsById: Map<number, TimeSlot>
  onOpen: (preset: Preset) => void
  onRename: (preset: Preset) => void
  onDuplicate: (preset: Preset) => void
  onDelete: (preset: Preset) => void
}

function relativeWhen(iso: string | null | undefined): string | null {
  if (!iso) return null
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return null
  const diff = Date.now() - date.getTime()
  const day = 24 * 60 * 60 * 1000
  if (diff < day) return "today"
  if (diff < 2 * day) return "yesterday"
  if (diff < 30 * day) return `${Math.floor(diff / day)}d ago`
  if (diff < 365 * day) return `${Math.floor(diff / (30 * day))}mo ago`
  return `${Math.floor(diff / (365 * day))}y ago`
}

export function PresetCard({
  preset,
  foodsById,
  slotsById,
  onOpen,
  onRename,
  onDuplicate,
  onDelete,
}: PresetCardProps) {
  const { t } = useTranslation()

  const distinctFoodIDs = useMemo(() => {
    const seen = new Set<number>()
    const out: number[] = []
    for (const plate of preset.plates) {
      for (const c of plate.components) {
        if (seen.has(c.food_id)) continue
        seen.add(c.food_id)
        out.push(c.food_id)
        if (out.length >= 4) return out
      }
    }
    return out
  }, [preset.plates])

  const distinctSlots = useMemo(() => {
    const seen = new Set<number>()
    const out: TimeSlot[] = []
    for (const plate of preset.plates) {
      if (seen.has(plate.slot_id)) continue
      const slot = slotsById.get(plate.slot_id)
      if (!slot) continue
      seen.add(plate.slot_id)
      out.push(slot)
    }
    return out
  }, [preset.plates, slotsById])

  const when = relativeWhen(preset.last_used_at ?? null)
  const totalFoods = preset.plates.reduce(
    (acc, plate) => acc + plate.components.length,
    0
  )
  const moreFoods = totalFoods - distinctFoodIDs.length

  return (
    <article
      className="group relative flex flex-col overflow-hidden rounded-lg border border-border bg-card transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-sm"
      data-testid={`preset-card-${preset.id}`}
    >
      <span
        aria-hidden="true"
        className="absolute inset-x-0 top-0 h-1 bg-accent"
      />
      <button
        type="button"
        onClick={() => onOpen(preset)}
        className="flex w-full flex-col items-start gap-3 px-5 pt-5 pb-4 text-left focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:outline-none"
      >
        <div className="flex w-full items-start justify-between gap-2">
          <h3 className="text-lg leading-tight font-semibold tracking-tight">
            {preset.name}
          </h3>
          <Badge variant="secondary" className="shrink-0 text-xs">
            {t("preset.plates_count", { count: preset.plates.length })}
          </Badge>
        </div>

        <div className="flex flex-wrap gap-1.5">
          {distinctFoodIDs.length === 0 ? (
            <span className="text-xs text-muted-foreground">
              {t("preset.components_count", { count: 0 })}
            </span>
          ) : (
            distinctFoodIDs.map((id) => (
              <Badge key={id} variant="outline" className="text-xs font-normal">
                {foodsById.get(id)?.name ?? `#${id}`}
              </Badge>
            ))
          )}
          {moreFoods > 0 && (
            <Badge variant="outline" className="text-xs font-normal">
              +{moreFoods}
            </Badge>
          )}
        </div>

        {(distinctSlots.length > 0 || preset.tags.length > 0) && (
          <div className="flex w-full flex-wrap items-center gap-1.5">
            {distinctSlots.slice(0, 3).map((slot) => (
              <span
                key={slot.id}
                className="font-body text-[10px] font-bold tracking-widest text-on-surface-variant uppercase"
              >
                {t(slot.name_key, { defaultValue: slot.name_key })}
              </span>
            ))}
            {preset.tags.length > 0 && (
              <span className="ml-auto flex flex-wrap gap-1">
                {preset.tags.slice(0, 3).map((tag) => (
                  <Badge
                    key={tag}
                    variant="outline"
                    className="text-[10px] font-normal"
                  >
                    {tag}
                  </Badge>
                ))}
              </span>
            )}
          </div>
        )}
      </button>
      <div className="flex items-center justify-between border-t border-dashed border-border/60 px-4 py-2">
        <span className="text-[11px] text-muted-foreground">
          {when ? t("preset.last_used", { when }) : t("preset.last_used_never")}
        </span>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("common.actions", { defaultValue: "Actions" })}
            >
              <MoreVertical className="size-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => onRename(preset)}>
              <Pencil className="size-3.5" />
              {t("preset.rename")}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => onDuplicate(preset)}>
              <Copy className="size-3.5" />
              {t("preset.duplicate")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onDelete(preset)}
              className="text-destructive focus:text-destructive"
            >
              <Trash2 className="size-3.5" />
              {t("preset.delete")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </article>
  )
}

export type { Food, Preset, TimeSlot }
