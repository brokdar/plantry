import {
  BookmarkPlus,
  MoreVertical,
  NotebookPen,
  Plus,
  RotateCcw,
  Trash2,
  Utensils,
  X,
} from "lucide-react"
import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"
import type { MacrosResponse } from "@/lib/api/plates"
import { cn } from "@/lib/utils"

import { AiFilledBadge } from "./AiFilledBadge"
import type { DragHandle } from "./DndCellWrapper"
import { SlotActions } from "./SlotActions"
import { SlotChips } from "./SlotChips"
import { SlotHero } from "./SlotHero"
import { SlotMacroDots } from "./SlotMacroDots"

interface SlotCellProps {
  day: number
  slotId: number
  plate: Plate | undefined
  componentsById: Map<number, Food>
  macros?: MacrosResponse
  aiFilled?: boolean
  /** dnd-kit drag handle for planned cells. The stretched-link button uses
   *  it as the drag activator so the cell remains a single interactive
   *  element for both click-to-open and drag-to-reschedule. */
  dragHandle?: DragHandle | null
  onAdd: () => void
  onOpenSheet?: () => void
  onDeletePlate: () => void
  onSaveAsTemplate?: () => void
  onToggleFavorite: () => void
  /** Toggle skip. Pass an explicit `note` (string or null) to set/clear the
   *  note alongside the toggle; omit to preserve the existing note. */
  onToggleSkip: (note?: string | null) => void
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

function EmptySlot({
  onAdd,
  onToggleSkip,
}: Pick<SlotCellProps, "onAdd" | "onToggleSkip">) {
  const { t } = useTranslation()
  const [popoverOpen, setPopoverOpen] = useState(false)

  function handleSkipKeyDown(e: React.KeyboardEvent) {
    if (e.key === "s" || e.key === "S") {
      e.preventDefault()
      onToggleSkip()
    }
  }

  return (
    <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
      <div
        data-slot-state="empty"
        onContextMenu={(e) => {
          e.preventDefault()
          setPopoverOpen(true)
        }}
        className={cn(
          CELL_HEIGHT,
          "group relative w-full rounded-[14px] border border-dashed border-outline-variant/40 bg-surface-container-low/50 transition-[border-color,background-color,box-shadow] duration-150 ease-out hover:border-primary/50 hover:bg-surface-container-low hover:shadow-sm"
        )}
      >
        <button
          type="button"
          onClick={onAdd}
          onKeyDown={handleSkipKeyDown}
          aria-label={t("planner.slot.empty.label")}
          data-testid="slot-empty-add"
          className="group/add absolute inset-0 flex flex-col items-center justify-center gap-2 rounded-[14px] text-on-surface-variant transition-transform duration-150 ease-out hover:-translate-y-px focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none focus-visible:ring-inset"
        >
          <span className="grid size-8 place-items-center rounded-full border border-outline-variant bg-surface-container-lowest text-on-surface transition-[background-color,border-color,color,transform] duration-150 ease-out group-hover/add:scale-110 group-hover/add:border-primary group-hover/add:bg-primary group-hover/add:text-on-primary">
            <Plus className="h-3.5 w-3.5" aria-hidden />
          </span>
          <span className="font-heading text-[9.5px] font-bold tracking-[0.18em] uppercase">
            {t("planner.slot.empty.label")}
          </span>
        </button>

        <TooltipProvider delayDuration={500}>
          <Tooltip>
            <TooltipTrigger asChild>
              <PopoverAnchor asChild>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    onToggleSkip()
                  }}
                  onKeyDown={handleSkipKeyDown}
                  onContextMenu={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    setPopoverOpen(true)
                  }}
                  aria-label={t("skip.skip_action")}
                  aria-keyshortcuts="S"
                  data-testid="slot-empty-skip"
                  className={cn(
                    "absolute right-1.5 bottom-1.5 z-[2] grid size-7 place-items-center rounded-full border border-outline-variant/60 bg-surface-container-lowest text-on-surface-variant/70 shadow-sm transition-[opacity,color,border-color,background-color,transform] duration-150 ease-out hover:-translate-y-px hover:border-tertiary/50 hover:bg-white hover:text-tertiary focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none",
                    // Subtle on desktop until hover, always-visible on touch.
                    "opacity-0 group-hover:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100"
                  )}
                >
                  <Utensils className="h-3 w-3" aria-hidden />
                </button>
              </PopoverAnchor>
            </TooltipTrigger>
            <TooltipContent side="left" sideOffset={4}>
              {t("skip.tooltip_empty")}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>

        <SkipNotePopoverContent
          initialNote=""
          title={t("skip.add_note_title")}
          onSubmit={(note) => {
            onToggleSkip(note.length > 0 ? note : null)
            setPopoverOpen(false)
          }}
          onCancel={() => setPopoverOpen(false)}
        />
      </div>
    </Popover>
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
  const [popoverOpen, setPopoverOpen] = useState(false)

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "s" || e.key === "S") {
      e.preventDefault()
      onToggleSkip()
    } else if (e.key === "Delete" || e.key === "Backspace") {
      e.preventDefault()
      onDeletePlate()
    }
  }

  return (
    <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
      <PopoverAnchor asChild>
        <div
          data-slot-state="skipped"
          role="group"
          tabIndex={0}
          onKeyDown={handleKeyDown}
          onContextMenu={(e) => {
            e.preventDefault()
            setPopoverOpen(true)
          }}
          aria-label={
            plate.note
              ? `${t("skip.eating_out")} — ${plate.note}`
              : t("skip.eating_out")
          }
          aria-keyshortcuts="S Delete"
          className={cn(
            CELL_HEIGHT,
            "group relative flex flex-col items-center justify-center gap-2 overflow-hidden rounded-[14px] border border-tertiary/30 px-3 text-center focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none focus-visible:ring-inset"
          )}
          style={{
            backgroundImage:
              "repeating-linear-gradient(45deg, transparent 0 7px, rgba(75,96,120,0.16) 7px 8px), var(--surface-container-low)",
          }}
        >
          {/* Big diagonal X overlay — purely decorative, painted under content.
              Fades in over ~250 ms when the cell first mounts as skipped
              (motion-safe so users with reduced-motion get an instant
              render). */}
          <svg
            aria-hidden
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
            className="pointer-events-none absolute inset-0 h-full w-full text-tertiary/35 motion-safe:animate-in motion-safe:duration-300 motion-safe:fade-in-0"
          >
            <line
              x1="8"
              y1="8"
              x2="92"
              y2="92"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              vectorEffect="non-scaling-stroke"
            />
            <line
              x1="92"
              y1="8"
              x2="8"
              y2="92"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              vectorEffect="non-scaling-stroke"
            />
          </svg>

          <span className="relative grid size-9 place-items-center rounded-full border border-tertiary/40 bg-white text-tertiary shadow-sm">
            <Utensils className="h-4 w-4" aria-hidden />
          </span>
          <span className="relative font-heading text-[11px] font-bold tracking-[0.2em] text-tertiary uppercase">
            {t("skip.eating_out")}
          </span>
          {plate.note && (
            <span
              className="relative line-clamp-2 max-w-full px-1 text-[12px] leading-snug text-on-surface"
              data-testid="slot-skip-note"
            >
              {plate.note}
            </span>
          )}

          {/* Action overlay — explicit edit-note + unskip + delete. Always
              rendered in the tab order at low opacity so keyboard focus
              never lands on an invisible target (WCAG 2.4.7). Pops to full
              opacity on hover, focus-within, or touch. */}
          <div className="absolute top-1.5 right-1.5 z-[2] flex items-center gap-1 opacity-50 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100">
            <TooltipProvider delayDuration={500}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    data-testid="slot-skip-edit-note"
                    className="h-7 w-7 rounded-full bg-white/90 text-on-surface-variant shadow-sm hover:text-primary"
                    aria-label={
                      plate.note
                        ? t("skip.edit_note")
                        : t("skip.add_note_title_existing")
                    }
                    onClick={(e) => {
                      e.stopPropagation()
                      setPopoverOpen(true)
                    }}
                  >
                    <NotebookPen className="h-3 w-3" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="bottom" sideOffset={4}>
                  {t("skip.tooltip_edit_note")}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <Button
              variant="ghost"
              size="icon"
              data-testid="slot-skip-unmark"
              className="h-7 w-7 rounded-full bg-white/90 text-on-surface-variant shadow-sm hover:text-tertiary"
              aria-label={t("skip.unmark")}
              onClick={(e) => {
                e.stopPropagation()
                onToggleSkip()
              }}
            >
              <RotateCcw className="h-3 w-3" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              data-testid="slot-quick-delete"
              className="h-7 w-7 rounded-full bg-white/90 text-destructive/70 shadow-sm hover:text-destructive"
              aria-label={t("plate.delete_plate")}
              onClick={(e) => {
                e.stopPropagation()
                onDeletePlate()
              }}
            >
              <X className="h-3 w-3" />
            </Button>
          </div>
        </div>
      </PopoverAnchor>

      <SkipNotePopoverContent
        initialNote={plate.note ?? ""}
        title={
          plate.note
            ? t("skip.edit_note_title")
            : t("skip.add_note_title_existing")
        }
        onSubmit={(note) => {
          onToggleSkip(note.length > 0 ? note : null)
          // Stay skipped — only the note changed. toggleSkip() in
          // lib/planner-skip.ts treats "noteOverride + already skipped" as
          // an edit and keeps skipped=true.
          setPopoverOpen(false)
        }}
        onCancel={() => setPopoverOpen(false)}
        skipOnly
      />
    </Popover>
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
  dragHandle,
  onAdd,
  onOpenSheet,
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
    return <EmptySlot onAdd={onAdd} onToggleSkip={onToggleSkip} />
  }

  const hero = sorted[0]
  const heroComp = componentsById.get(hero.food_id)
  const sideComps = sorted.slice(1).map((pc) => {
    const c = componentsById.get(pc.food_id)
    return c?.name ?? `#${pc.food_id}`
  })
  const heroName = heroComp?.name ?? `#${hero.food_id}`
  const heroRole =
    heroComp?.kind === "composed" ? (heroComp.role ?? null) : null
  const favorite = heroComp?.favorite ?? false
  const loved = plate.feedback?.status === "loved"
  const disliked = plate.feedback?.status === "disliked"

  function handleStretchedKeyDown(e: React.KeyboardEvent) {
    if (e.key === "s" || e.key === "S") {
      e.preventDefault()
      onToggleSkip()
    }
  }

  return (
    <div
      data-slot-state="planned"
      className={cn(
        CELL_HEIGHT,
        "group relative flex flex-col overflow-hidden rounded-[14px] border border-outline-variant/50 bg-surface-container-lowest transition-[border-color,box-shadow,transform] duration-150 ease-out hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-[0_2px_8px_rgba(25,28,28,0.06),0_4px_16px_-4px_rgba(74,101,77,0.14)]",
        aiFilled &&
          "border-[#c2974a]/40 shadow-[0_0_0_1px_rgba(194,151,74,0.3),0_4px_12px_-4px_rgba(25,28,28,0.06)]"
      )}
    >
      {/* Stretched-link click target. z-[1] puts it ABOVE the SlotHero
        (positioned z-auto, paint group 6) so clicks on the dish photo also
        open the sheet — z-0 would lose the tree-order tiebreak. The action
        row inside the body uses z-10, sitting above this button so its
        controls remain hit-testable.
        This same button is also the dnd-kit drag activator (when the cell
        is draggable), unifying click-to-open and drag-to-reschedule on a
        single interactive element so the a11y tree stays flat — one
        button per cell, not nested. */}
      {onOpenSheet && (
        <button
          type="button"
          onClick={onOpenSheet}
          onKeyDown={handleStretchedKeyDown}
          aria-label={t("slot_sheet.open_label")}
          data-testid="slot-open-sheet"
          ref={dragHandle?.setActivatorNodeRef}
          {...(dragHandle?.listeners ?? {})}
          {...(dragHandle?.attributes ?? {})}
          className="absolute inset-0 z-[1] cursor-pointer rounded-[14px] focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none focus-visible:ring-inset"
        />
      )}
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
        roleLabel={
          heroRole
            ? t(`planner.slot.role.${heroRole}`, { defaultValue: heroRole })
            : t("ingredient.kind_label")
        }
      />
      <div className="flex min-h-0 flex-1 flex-col gap-1 px-2.5 py-2">
        <div className="flex items-start justify-between gap-1">
          <span className="truncate font-heading text-[13.5px] leading-tight font-bold tracking-tight text-on-surface">
            {heroName}
          </span>
          <div className="relative z-10 flex shrink-0 items-center">
            <Button
              variant="ghost"
              size="icon"
              data-testid="slot-quick-delete"
              className="size-5 text-on-surface-variant/50 opacity-0 transition-opacity group-hover:opacity-100 hover:text-destructive"
              aria-label={t("plate.delete_plate")}
              onClick={onDeletePlate}
            >
              <X className="h-3 w-3" />
            </Button>
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
                <DropdownMenuItem onClick={() => onToggleSkip()}>
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
        </div>
        <SlotChips names={sideComps} max={3} />
        <div className="mt-auto">
          <SlotMacroDots macros={macros} />
        </div>
      </div>
    </div>
  )
}

/**
 * Inline note-editor body of a Popover. Caller wraps it with a `<Popover>`
 * containing a `<PopoverAnchor>` so the popover positions next to the relevant
 * skip-related control. Uses an uncontrolled input keyed by mount so it
 * resets cleanly on each open (Radix unmounts content when closed).
 */
function SkipNotePopoverContent({
  initialNote,
  title,
  onSubmit,
  onCancel,
  skipOnly = false,
}: {
  initialNote: string
  title: string
  onSubmit: (note: string) => void
  onCancel: () => void
  /** When true, render only "Save" as the primary action (used in the
   *  already-skipped state where Save just updates the note). When false,
   *  the primary action says "Skip with note" since clicking it both skips
   *  the slot AND saves the note. */
  skipOnly?: boolean
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const note = inputRef.current?.value.trim() ?? ""
    onSubmit(note)
  }

  return (
    <PopoverContent
      align="end"
      side="top"
      sideOffset={6}
      className="w-72"
      onOpenAutoFocus={(e) => {
        // Keep our own focus on the input so cursor lands on the text.
        e.preventDefault()
        inputRef.current?.focus()
        inputRef.current?.select()
      }}
      onClick={(e) => e.stopPropagation()}
      onContextMenu={(e) => e.stopPropagation()}
      data-testid="skip-note-popover"
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-2.5">
        <div className="font-heading text-[10px] font-bold tracking-[0.18em] text-on-surface-variant uppercase">
          {title}
        </div>
        <input
          ref={inputRef}
          defaultValue={initialNote}
          placeholder={t("skip.note.placeholder")}
          maxLength={200}
          data-testid="skip-note-input"
          className="w-full rounded-md border border-outline-variant bg-surface-container-lowest px-2.5 py-1.5 text-sm transition-[border-color,box-shadow] outline-none focus:border-primary focus:ring-2 focus:ring-primary/30"
        />
        <div className="flex justify-end gap-1.5">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onCancel}
            data-testid="skip-note-cancel"
          >
            {t("common.cancel")}
          </Button>
          <Button type="submit" size="sm" data-testid="skip-note-save">
            {skipOnly ? t("skip.save_note") : t("skip.skip_with_note")}
          </Button>
        </div>
      </form>
    </PopoverContent>
  )
}
