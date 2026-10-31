import {
  Bookmark as BookmarkIcon,
  BookmarkPlus,
  GripVertical,
  MoreVertical,
  NotebookPen,
  Plus,
  Trash2,
  Utensils,
  X,
} from "lucide-react"
import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { usePlannerUI } from "@/lib/stores/planner-ui"

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
import type { AvailabilityCell } from "@/lib/api/availability"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"
import type { MacrosResponse } from "@/lib/api/plates"
import { cn } from "@/lib/utils"

import { EmptySlotPresetPicker } from "@/components/presets/EmptySlotPresetPicker"

import { AiFilledBadge } from "./AiFilledBadge"
import { SlotActions } from "./SlotActions"
import { SlotHero, type SlotHeroComponent } from "./SlotHero"
import { SlotMacroDots } from "./SlotMacroDots"
import { SlotSides, type SlotSideItem } from "./SlotSides"

interface SlotCellProps {
  day: number
  slotId: number
  /** ISO date string (YYYY-MM-DD) for this cell. Optional so existing call
   *  sites that don't surface a preset picker keep compiling; when present,
   *  the empty-slot picker uses it as the apply target. */
  date?: string
  slotName?: string
  plate: Plate | undefined
  componentsById: Map<number, Food>
  macros?: MacrosResponse
  /** Per-slot kcal share of the daily target. Drives the macro-target ring on
   *  the planned cell; omit to hide the indicator. */
  kcalTarget?: number | null
  aiFilled?: boolean
  /** When false, suppress the kcal + macro dots row at the bottom of the
   *  planted cell. The desktop weekly view sets this to false to keep the
   *  cell visually quiet; the mobile day-tab view keeps the default so
   *  numbers stay accessible on that single-column layout. */
  showMacros?: boolean
  onAdd: () => void
  onOpenSheet?: () => void
  onDeletePlate: () => void
  onSaveAsTemplate?: () => void
  onSaveAsPreset?: () => void
  onToggleFavorite: () => void
  /** The cell's resolved availability. A `none` cell without a plate renders
   *  the No plate state; a `routine` one names its routine. */
  availability?: AvailabilityCell
  /** Mark the cell No plate, optionally with a note (removes its plate). */
  onMarkNoPlate: (note?: string | null) => void
  /** No plate cell: make the meal plannable again. */
  onPlanMeal?: () => void
  /** No plate cell: set or clear its note. */
  onEditNote?: (note: string | null) => void
  onRateLoved: () => void
  onRateDisliked: () => void
}

// All states share the 178 px standard so rows scan as a single line of
// ledger across the week. Per-state height variance was tried (empty 178,
// planned 200, no plate 120) but the planted-cell macro line + ring was
// never wired (no per-plate macros endpoint), leaving an ~80 px dead band
// at the bottom of every planted cell. Until per-plate macros are fetched,
// uniform height keeps the row honest.
const CELL_HEIGHT = "h-[178px]"
// The kcal row (showMacros, phone day view only) needs 16 px more; that view
// stacks one cell per row, so the taller cell breaks no grid alignment.
const CELL_HEIGHT_WITH_MACROS = "h-[194px]"

export function SlotCell(props: SlotCellProps) {
  const { plate, availability } = props

  if (plate) return <PlannedSlot {...props} plate={plate} />
  if (availability?.mode === "none") {
    return <NoPlateSlot {...props} availability={availability} />
  }
  return <EmptySlot {...props} />
}

function isMarkNoPlateKey(e: React.KeyboardEvent) {
  return e.key === "n" || e.key === "N"
}

function EmptySlot({
  onAdd,
  onMarkNoPlate,
  availability,
  date,
  slotId,
  slotName,
}: Pick<
  SlotCellProps,
  "onAdd" | "onMarkNoPlate" | "availability" | "date" | "slotName"
> & {
  slotId?: number
}) {
  const { t } = useTranslation()
  const [popoverOpen, setPopoverOpen] = useState(false)

  function handleMarkKeyDown(e: React.KeyboardEvent) {
    if (isMarkNoPlateKey(e)) {
      e.preventDefault()
      onMarkNoPlate()
    }
  }

  // D8: a routine is a label in Phase 1; the cell still takes a plate by hand.
  const routineLabel =
    availability?.mode === "routine"
      ? availability.preset_name
        ? t("availability.routine_named", { name: availability.preset_name })
        : t("availability.mode.routine")
      : null

  return (
    <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
      <div
        data-slot-state="empty"
        data-availability={availability?.mode}
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
          onKeyDown={handleMarkKeyDown}
          aria-label={
            routineLabel
              ? `${t("planner.slot.empty.label")}, ${routineLabel}`
              : t("planner.slot.empty.label")
          }
          aria-keyshortcuts="N"
          data-testid="slot-empty-add"
          data-cell-focus=""
          className="group/add absolute inset-0 flex flex-col items-center justify-center gap-2 rounded-[14px] text-on-surface-variant transition-transform duration-150 ease-out hover:-translate-y-px focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none focus-visible:ring-inset"
        >
          <span className="grid size-8 place-items-center rounded-full border border-outline-variant bg-surface-container-lowest text-on-surface transition-[background-color,border-color,color,transform] duration-150 ease-out group-hover/add:scale-110 group-hover/add:border-primary group-hover/add:bg-primary group-hover/add:text-on-primary">
            <Plus className="h-3.5 w-3.5" aria-hidden />
          </span>
          <span className="font-heading text-[9.5px] font-bold tracking-[0.18em] uppercase">
            {t("planner.slot.empty.label")}
          </span>
          {routineLabel && (
            <span
              className="max-w-full truncate px-2 text-small text-ink-muted"
              data-testid="slot-routine-label"
            >
              {routineLabel}
            </span>
          )}
        </button>

        <TooltipProvider delayDuration={500}>
          <Tooltip>
            <TooltipTrigger asChild>
              <PopoverAnchor asChild>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    onMarkNoPlate()
                  }}
                  onKeyDown={handleMarkKeyDown}
                  onContextMenu={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    setPopoverOpen(true)
                  }}
                  aria-label={t("planner.availability.mark")}
                  aria-keyshortcuts="N"
                  data-testid="slot-empty-no-plate"
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
              {t("planner.availability.mark_tooltip")}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>

        {date != null && slotId != null && (
          <EmptySlotPresetPicker
            slotId={slotId}
            slotLabel={slotName ?? ""}
            targetDate={date}
            trigger={
              <button
                type="button"
                aria-label={t("preset.empty_slot_picker.title", {
                  slot: slotName ?? "",
                })}
                data-testid="slot-empty-preset-trigger"
                className={cn(
                  "absolute right-1.5 bottom-9 z-[2] grid size-7 place-items-center rounded-full border border-outline-variant/60 bg-surface-container-lowest text-on-surface-variant/70 shadow-sm transition-[opacity,color,border-color,background-color,transform] duration-150 ease-out hover:-translate-y-px hover:border-primary/50 hover:bg-white hover:text-primary focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none",
                  "opacity-0 group-hover:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100"
                )}
              >
                <BookmarkIcon className="h-3 w-3" aria-hidden />
              </button>
            }
          />
        )}

        <NoPlateNotePopoverContent
          initialNote=""
          title={t("planner.availability.mark_with_note_title")}
          submitLabel={t("planner.availability.mark")}
          onSubmit={(note) => {
            onMarkNoPlate(note.length > 0 ? note : null)
            setPopoverOpen(false)
          }}
          onCancel={() => setPopoverOpen(false)}
        />
      </div>
    </Popover>
  )
}

/**
 * A meal the user won't eat at home (availability `none`, UX §5.5): a quiet,
 * transparent italic "No plate" with its reason. There is no plate here, so
 * the only actions are making the meal plannable again and editing the note.
 */
function NoPlateSlot({
  availability,
  onPlanMeal,
  onEditNote,
}: SlotCellProps & { availability: AvailabilityCell }) {
  const { t } = useTranslation()
  const [popoverOpen, setPopoverOpen] = useState(false)
  const note = availability.note
  const label = t("planner.availability.no_plate")
  const noteLabel = note
    ? t("planner.availability.edit_note")
    : t("planner.availability.add_note")

  return (
    <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
      <PopoverAnchor asChild>
        <div
          data-slot-state="none"
          data-cell-focus=""
          role="group"
          tabIndex={0}
          aria-label={note ? `${label}, ${note}` : label}
          onContextMenu={(e) => {
            e.preventDefault()
            setPopoverOpen(true)
          }}
          className={cn(
            CELL_HEIGHT,
            "flex flex-col items-center justify-center gap-1 rounded-cell border border-dashed border-line bg-transparent px-2 text-center focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          )}
        >
          <span className="text-body text-ink-muted italic">{label}</span>
          {note && (
            <span
              className="line-clamp-2 max-w-full text-small text-ink"
              data-testid="slot-no-plate-note"
            >
              {note}
            </span>
          )}
          <div className="mt-2 flex flex-col items-stretch gap-0.5">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onPlanMeal}
              data-testid="slot-no-plate-plan"
              className="min-h-11 text-small text-accent hover:bg-panel-sunken hover:text-accent pointer-fine:min-h-7"
            >
              <Plus aria-hidden />
              {t("planner.availability.plan_meal")}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setPopoverOpen(true)}
              data-testid="slot-no-plate-edit-note"
              className="min-h-11 text-small text-ink-muted hover:bg-panel-sunken hover:text-ink pointer-fine:min-h-7"
            >
              <NotebookPen aria-hidden />
              {noteLabel}
            </Button>
          </div>
        </div>
      </PopoverAnchor>

      <NoPlateNotePopoverContent
        initialNote={note ?? ""}
        title={noteLabel}
        submitLabel={t("planner.availability.save_note")}
        onSubmit={(next) => {
          onEditNote?.(next.length > 0 ? next : null)
          setPopoverOpen(false)
        }}
        onCancel={() => setPopoverOpen(false)}
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
  kcalTarget,
  aiFilled,
  showMacros = true,
  onAdd,
  onOpenSheet,
  onDeletePlate,
  onSaveAsTemplate,
  onSaveAsPreset,
  onToggleFavorite,
  onMarkNoPlate,
  availability,
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
    return (
      <EmptySlot
        onAdd={onAdd}
        onMarkNoPlate={onMarkNoPlate}
        availability={availability}
      />
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
  const heroComponent = componentsById.get(sorted[0]!.food_id)
  const heroName = componentMeta[0]!.name
  const heroRoleKey = componentMeta[0]!.role
  const heroComponents: SlotHeroComponent[] = componentMeta
  const sideItems: SlotSideItem[] = componentMeta.slice(1)
  const favorite = heroComponent?.favorite ?? false
  const loved = plate.feedback?.status === "loved"
  const disliked = plate.feedback?.status === "disliked"

  function handleStretchedKeyDown(e: React.KeyboardEvent) {
    if (isMarkNoPlateKey(e)) {
      e.preventDefault()
      onMarkNoPlate()
    } else if (e.key === "Delete" || e.key === "Backspace") {
      e.preventDefault()
      onDeletePlate()
    }
  }

  return (
    <div
      data-slot-state="planned"
      className={cn(
        showMacros ? CELL_HEIGHT_WITH_MACROS : CELL_HEIGHT,
        "group relative flex cursor-grab flex-col overflow-hidden rounded-[14px] border border-outline-variant/50 bg-surface-container-lowest transition-[border-color,box-shadow,transform] duration-150 ease-out hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-[0_2px_8px_rgba(25,28,28,0.06),0_4px_16px_-4px_rgba(74,101,77,0.14)] active:cursor-grabbing",
        aiFilled &&
          "border-ai-accent/40 shadow-[0_0_0_1px_rgba(194,151,74,0.3),0_4px_12px_-4px_rgba(25,28,28,0.06)]"
      )}
    >
      {/* Stretched-link click target. z-[1] puts it ABOVE the SlotHero
        (positioned z-auto, paint group 6) so clicks on the dish photo also
        open the sheet — z-0 would lose the tree-order tiebreak. The action
        row inside the body uses z-10, sitting above this button so its
        controls remain hit-testable. Drag activation happens on the parent
        DndCellWrapper via dnd-kit listeners + a 6px PointerSensor distance
        constraint; short clicks land here, longer drags activate dnd-kit. */}
      {onOpenSheet && (
        <button
          type="button"
          onClick={onOpenSheet}
          onKeyDown={handleStretchedKeyDown}
          aria-label={t("slot_sheet.open_label")}
          aria-keyshortcuts="N Delete"
          data-testid="slot-open-sheet"
          data-cell-focus=""
          className="absolute inset-0 z-[1] cursor-pointer rounded-[14px] focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none focus-visible:ring-inset"
        />
      )}
      <DragGripIndicator />
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
        components={heroComponents}
        heroRoleLabel={
          heroRoleKey
            ? t(`planner.slot.role.${heroRoleKey}`, {
                defaultValue: heroRoleKey,
              })
            : null
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
                <DropdownMenuItem onClick={() => onMarkNoPlate()}>
                  <Utensils className="h-3 w-3" />
                  {t("planner.availability.mark")}
                </DropdownMenuItem>
                {onSaveAsTemplate && (
                  <DropdownMenuItem onClick={onSaveAsTemplate}>
                    <BookmarkPlus className="h-3 w-3" />
                    {t("template.save_as")}
                  </DropdownMenuItem>
                )}
                {onSaveAsPreset && (
                  <DropdownMenuItem onClick={onSaveAsPreset}>
                    <BookmarkPlus className="h-3 w-3" />
                    {t("preset.save_as")}
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
        <SlotSides items={sideItems} max={3} />
        {showMacros && (
          <div className="mt-auto">
            <SlotMacroDots macros={macros} kcalTarget={kcalTarget} />
          </div>
        )}
      </div>
    </div>
  )
}

/**
 * Visual-only affordance for the drag-to-reschedule gesture. The whole cell
 * is the actual drag activator (listeners live on DndCellWrapper); this grip
 * just hints at it on hover, alongside a "⌘ to copy" tooltip until the user
 * has done their first ⌘+drag (persisted via localStorage in planner-ui).
 * pointer-events-none keeps it from intercepting the stretched-link click.
 */
function DragGripIndicator() {
  const { t } = useTranslation()
  const copyHintSeen = usePlannerUI((s) => s.copyHintSeen)
  return (
    <div
      aria-hidden
      data-testid="slot-drag-grip"
      className="pointer-events-none absolute top-2 bottom-2 left-0 z-[2] flex w-3 items-center justify-center text-on-surface-variant/40 opacity-0 transition-opacity duration-150 group-hover:opacity-100"
    >
      <GripVertical className="h-3.5 w-3.5" />
      {!copyHintSeen && (
        <span
          className="absolute top-full left-1/2 mt-1 -translate-x-1/2 rounded-md bg-on-surface px-1.5 py-0.5 font-heading text-[9px] font-bold tracking-[0.12em] whitespace-nowrap text-on-primary uppercase shadow-sm"
          data-testid="slot-copy-hint"
        >
          {t("planner.dnd.copy_hint")}
        </span>
      )}
    </div>
  )
}

/**
 * Inline note-editor body of a Popover. Caller wraps it with a `<Popover>`
 * containing a `<PopoverAnchor>` so the popover positions next to the cell.
 * Uses an uncontrolled input keyed by mount so it resets cleanly on each
 * open (Radix unmounts content when closed).
 */
function NoPlateNotePopoverContent({
  initialNote,
  title,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initialNote: string
  title: string
  submitLabel: string
  onSubmit: (note: string) => void
  onCancel: () => void
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
      data-testid="no-plate-note-popover"
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-2.5">
        <div className="text-small font-semibold text-ink">{title}</div>
        <input
          ref={inputRef}
          defaultValue={initialNote}
          placeholder={t("planner.availability.note_placeholder")}
          aria-label={title}
          maxLength={200}
          data-testid="no-plate-note-input"
          className="w-full rounded-cell border border-line bg-panel px-2.5 py-1.5 text-body text-ink outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        />
        <div className="flex justify-end gap-1.5">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onCancel}
            data-testid="no-plate-note-cancel"
          >
            {t("common.cancel")}
          </Button>
          <Button type="submit" size="sm" data-testid="no-plate-note-save">
            {submitLabel}
          </Button>
        </div>
      </form>
    </PopoverContent>
  )
}
