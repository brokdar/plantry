import {
  BookmarkPlus,
  MoreVertical,
  Plus,
  Trash2,
  Utensils,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Component } from "@/lib/api/components"
import type { Plate } from "@/lib/api/plates"
import type { MacrosResponse } from "@/lib/api/weeks"
import { cn } from "@/lib/utils"

import { AiFilledBadge } from "./AiFilledBadge"
import { SlotActions } from "./SlotActions"
import { SlotChips } from "./SlotChips"
import { SlotHero } from "./SlotHero"
import { SlotMacroDots } from "./SlotMacroDots"

interface SlotCellProps {
  day: number
  slotId: number
  plate: Plate | undefined
  componentsById: Map<number, Component>
  macros?: MacrosResponse
  aiFilled?: boolean
  onAdd: () => void
  onDeletePlate: () => void
  onSaveAsTemplate?: () => void
  onToggleFavorite: () => void
  onToggleSkip: () => void
  onRateLoved: () => void
  onRateDisliked: () => void
}

// SlotCell fixes a 178px height so the week grid never jitters with content
// differences. Every state (planned / empty / skipped) shares the same outer
// frame so rows align perfectly.
const CELL_HEIGHT = "h-[178px]"

export function SlotCell(props: SlotCellProps) {
  const { plate } = props

  if (!plate) return <EmptySlot {...props} />
  if (plate.skipped) return <SkippedSlot {...props} plate={plate} />
  return <PlannedSlot {...props} plate={plate} />
}

function EmptySlot({ onAdd }: SlotCellProps) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      onClick={onAdd}
      aria-label={t("planner.slot.empty.label")}
      data-slot-state="empty"
      className={cn(
        CELL_HEIGHT,
        "group flex w-full flex-col items-center justify-center gap-2 rounded-[14px] border border-dashed border-outline-variant/40 bg-surface-container-low/50 text-on-surface-variant transition-colors hover:border-primary/50 hover:bg-surface-container-low"
      )}
    >
      <span className="grid size-8 place-items-center rounded-full border border-outline-variant bg-surface-container-lowest text-on-surface group-hover:border-primary group-hover:bg-primary group-hover:text-on-primary">
        <Plus className="h-3.5 w-3.5" aria-hidden />
      </span>
      <span className="font-heading text-[9.5px] font-bold tracking-[0.18em] uppercase">
        {t("planner.slot.empty.label")}
      </span>
    </button>
  )
}

interface SkippedSlotPropsExt extends SlotCellProps {
  plate: Plate
}

function SkippedSlot({
  plate,
  onToggleSkip,
  onDeletePlate,
}: SkippedSlotPropsExt) {
  const { t } = useTranslation()
  return (
    <div
      data-slot-state="skipped"
      className={cn(
        CELL_HEIGHT,
        "group relative flex flex-col items-center justify-center gap-1.5 overflow-hidden rounded-[14px] border border-tertiary/25 p-3 text-center"
      )}
      style={{
        backgroundImage:
          "repeating-linear-gradient(45deg, transparent 0 7px, rgba(75,96,120,0.16) 7px 8px), var(--surface-container-low)",
      }}
    >
      <button
        type="button"
        onClick={onToggleSkip}
        aria-label={t("skip.unmark")}
        data-testid="slot-skip-toggle"
        className="absolute inset-0 cursor-pointer"
      >
        <span className="sr-only">{t("skip.unmark")}</span>
      </button>
      <span className="relative grid size-[30px] place-items-center rounded-full border border-tertiary/40 bg-white text-tertiary">
        <Utensils className="h-3.5 w-3.5" aria-hidden />
      </span>
      <span className="relative font-heading text-[10px] font-bold tracking-[0.18em] text-tertiary uppercase">
        {t("skip.label")}
      </span>
      {plate.note && (
        <span className="relative max-w-full truncate text-[11px] text-on-tertiary-fixed-variant italic">
          {plate.note}
        </span>
      )}
      <div className="absolute top-2 right-2 opacity-0 transition-opacity group-hover:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 rounded-full bg-white/90 shadow-sm"
              aria-label={t("common.actions")}
              onClick={(e) => e.stopPropagation()}
            >
              <MoreVertical className="h-3 w-3" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
            <DropdownMenuItem onClick={onToggleSkip}>
              {t("skip.unmark")}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={onDeletePlate}
              className="text-destructive"
            >
              <Trash2 className="h-3 w-3" />
              {t("plate.delete_plate")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}

interface PlannedSlotPropsExt extends SlotCellProps {
  plate: Plate
}

function PlannedSlot({
  plate,
  componentsById,
  macros,
  aiFilled,
  onAdd,
  onDeletePlate,
  onSaveAsTemplate,
  onToggleFavorite,
  onToggleSkip,
  onRateLoved,
  onRateDisliked,
}: PlannedSlotPropsExt) {
  const { t } = useTranslation()

  // "First by sort_order wins hero" — matches the user decision in the design
  // review. When no components are attached we fall back to the empty state
  // shape so the card never renders with a blank title.
  const sorted = [...plate.components].sort(
    (a, b) => a.sort_order - b.sort_order
  )
  if (sorted.length === 0) {
    return <EmptySlot {...({ onAdd } as unknown as SlotCellProps)} />
  }

  const hero = sorted[0]
  const heroComp = componentsById.get(hero.component_id)
  const sideComps = sorted.slice(1).map((pc) => {
    const c = componentsById.get(pc.component_id)
    return c?.name ?? `#${pc.component_id}`
  })
  const heroName = heroComp?.name ?? `#${hero.component_id}`
  const heroRole = heroComp?.role ?? "main"
  const favorite = heroComp?.favorite ?? false
  const loved = plate.feedback?.status === "loved"
  const disliked = plate.feedback?.status === "disliked"

  return (
    <div
      data-slot-state="planned"
      className={cn(
        CELL_HEIGHT,
        "group relative flex flex-col overflow-hidden rounded-[14px] border border-outline-variant/50 bg-surface-container-lowest transition-[border-color,box-shadow] hover:border-primary/40 hover:shadow-[0_1px_2px_rgba(25,28,28,0.04),0_4px_12px_-4px_rgba(25,28,28,0.06)]",
        aiFilled &&
          "border-[#c2974a]/40 shadow-[0_0_0_1px_rgba(194,151,74,0.3),0_4px_12px_-4px_rgba(25,28,28,0.06)]"
      )}
    >
      {aiFilled && <AiFilledBadge />}
      <SlotActions
        favorite={favorite}
        loved={loved}
        disliked={disliked}
        onFavorite={onToggleFavorite}
        onLove={onRateLoved}
        onDislike={onRateDisliked}
      />
      <SlotHero
        imagePath={heroComp?.image_path}
        role={heroRole}
        roleLabel={t(`planner.slot.role.${heroRole}`, {
          defaultValue: heroRole,
        })}
      />
      <div className="flex min-h-0 flex-1 flex-col gap-1 px-2.5 py-2">
        <div className="flex items-start justify-between gap-1">
          <span className="truncate font-heading text-[13.5px] leading-tight font-bold tracking-tight text-on-surface">
            {heroName}
          </span>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="-mr-1 size-5 shrink-0"
                aria-label={t("common.actions")}
              >
                <MoreVertical className="h-3 w-3" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={onAdd}>
                <Plus className="h-3 w-3" />
                {t("plate.add_component")}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={onToggleSkip}>
                {t("skip.mark")}
              </DropdownMenuItem>
              {onSaveAsTemplate && (
                <DropdownMenuItem onClick={onSaveAsTemplate}>
                  <BookmarkPlus className="h-3 w-3" />
                  {t("template.save_as")}
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={onDeletePlate}
                className="text-destructive"
              >
                <Trash2 className="h-3 w-3" />
                {t("plate.delete_plate")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <SlotChips names={sideComps} max={3} />
        <div className="mt-auto">
          <SlotMacroDots macros={macros} />
        </div>
      </div>
    </div>
  )
}
