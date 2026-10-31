import { useDraggable, useDroppable } from "@dnd-kit/core"
import { Lock, Plus, RefreshCw, Shuffle, TriangleAlert } from "lucide-react"
import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import { useBatchCell } from "@/components/batch/batch-highlight"
import type { BoardCell } from "@/lib/plan/board"
import { BATCH_CLASSES, BATCH_HIGHLIGHT_CLASS } from "@/lib/plan/batch-colours"
import { cn } from "@/lib/utils"

export type PlateCellLayout = "grid" | "list"

interface PlateCellProps {
  cell: BoardCell
  layout: PlateCellLayout
  /** Past windows: no "Add", no dragging, no dropping. */
  readOnly: boolean
  /** Drag and drop is on (desktop and tablet grid). */
  dnd: boolean
  onOpen: (cell: BoardCell) => void
  /** Desktop hover quick actions on a draft plate (D33); without them the
   *  cell shows none. */
  onReplace?: (cell: BoardCell) => void
  onDifferentSide?: (cell: BoardCell) => void
  /** "row-col" for keyboard navigation (grid only). */
  pos?: string
  /** Roving tabindex: whether this cell is the grid's tab stop. */
  tabStop?: boolean
  onFocusCell?: (key: string) => void
}

/** Visual state of the cell's surface (UX §4, §5.5). A draft plate has a
 *  dashed border instead of the solid ring (a ring can't be dashed). */
function surfaceClass(cell: BoardCell): string {
  const draft = cell.state === "draft"
  if (cell.badge?.variant === "cook") {
    // Tint means "cooked here"; the 1.5 px ink outline is an inset ring so
    // the batch highlight (an outer ring) and the focus outline stay free.
    return cn(
      BATCH_CLASSES[cell.badge.colour].tint,
      draft
        ? "border-[1.5px] border-dashed border-ink"
        : "inset-ring-[1.5px] inset-ring-ink"
    )
  }
  if (draft) return "bg-panel border border-dashed border-ink-muted"
  if (cell.plate) return "bg-panel inset-ring inset-ring-line"
  if (cell.mode === "none") return "bg-transparent"
  return "bg-panel inset-ring inset-ring-line"
}

/**
 * One board cell in every state: cook, leftover, plain plate, No plate,
 * routine and empty. It renders only from the `BoardCell` view model; the
 * button opens the cell (`onOpen`), and with `dnd` the plate drags and the
 * cell takes drops.
 */
export function PlateCell({
  cell,
  layout,
  readOnly,
  dnd,
  onOpen,
  onReplace,
  onDifferentSide,
  pos,
  tabStop,
  onFocusCell,
}: PlateCellProps) {
  const batch = useBatchCell(cell.sessionIds, cell.cookOf)
  const editable = dnd && !readOnly
  const { setNodeRef: setDropNode, isOver } = useDroppable({
    id: `cell:${cell.key}`,
    data: { cell },
    disabled: !editable,
  })
  const draggable = editable && !!cell.plate
  const {
    setNodeRef: setDragNode,
    listeners,
    transform,
    isDragging,
  } = useDraggable({
    id: `plate:${cell.key}`,
    data: { cell },
    disabled: !draggable,
  })

  const grid = layout === "grid"
  const quick =
    grid &&
    !readOnly &&
    cell.state === "draft" &&
    (onReplace || onDifferentSide)

  return (
    <div
      role={grid ? "gridcell" : undefined}
      ref={(el) => {
        setDropNode(el)
        return batch.ref(el)
      }}
      data-cell-pos={pos}
      data-cell-key={cell.key}
      data-cell-mode={cell.mode}
      data-cell-empty={cell.plate ? undefined : ""}
      data-highlighted={batch["data-highlighted"]}
      data-over={isOver && !isDragging ? "" : undefined}
      className={cn(
        "group/cell relative min-w-0 rounded-cell focus-within:z-20",
        BATCH_HIGHLIGHT_CLASS
      )}
    >
      <button
        type="button"
        ref={setDragNode}
        // Only the pointer listeners: the cell stays a plain button with its
        // own name and the grid's roving tabindex (no dnd-kit attributes).
        {...(draggable ? listeners : {})}
        style={
          transform
            ? {
                transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
              }
            : undefined
        }
        aria-label={cell.ariaLabel}
        tabIndex={tabStop === undefined ? undefined : tabStop ? 0 : -1}
        data-cell-focus=""
        data-testid="plan-cell"
        onFocus={() => onFocusCell?.(cell.key)}
        onClick={() => onOpen(cell)}
        className={cn(
          "flex w-full min-w-0 flex-col items-start gap-1 rounded-cell p-2 text-left transition-colors duration-150 ease-out",
          grid ? "h-full min-h-24" : "min-h-14",
          surfaceClass(cell),
          !cell.plate &&
            cell.mode !== "none" &&
            !readOnly &&
            "hover:bg-panel-sunken",
          cell.mode === "none" && "hover:inset-ring hover:inset-ring-line",
          isDragging && "relative z-30 opacity-80",
          isOver && !isDragging && "bg-accent/10 inset-ring-2 inset-ring-accent"
        )}
      >
        <CellContent cell={cell} readOnly={readOnly} />
      </button>
      {cell.attention && (
        <TriangleAlert
          data-testid="cell-warning"
          className="pointer-events-none absolute top-1.5 right-1.5 size-3.5 text-warning"
          strokeWidth={2}
          aria-hidden="true"
        />
      )}
      {cell.state === "confirmed" && (
        <Lock
          data-testid="cell-padlock"
          className="pointer-events-none absolute right-1.5 bottom-1.5 size-3 text-ink-muted"
          strokeWidth={2}
          aria-hidden="true"
        />
      )}
      {quick && (
        <QuickActions
          cell={cell}
          onReplace={onReplace}
          onDifferentSide={onDifferentSide}
        />
      )}
    </div>
  )
}

/**
 * Replace and Different side on hover (fine pointers only: touch reaches
 * them in the plate sheet). Out of the tab order: the keyboard has R and S.
 */
function QuickActions({
  cell,
  onReplace,
  onDifferentSide,
}: {
  cell: BoardCell
  onReplace?: (cell: BoardCell) => void
  onDifferentSide?: (cell: BoardCell) => void
}) {
  const { t } = useTranslation()
  const button =
    "grid size-7 place-items-center rounded-cell border border-line bg-panel text-ink hover:bg-panel-sunken"
  return (
    <div
      data-testid="cell-quick-actions"
      className="absolute right-1 bottom-1 hidden gap-1 opacity-0 transition-opacity duration-150 ease-out group-focus-within/cell:opacity-100 group-hover/cell:opacity-100 pointer-fine:flex"
    >
      {onDifferentSide && cell.sideComponentId !== null && (
        <button
          type="button"
          tabIndex={-1}
          title={t("plan.review.different_side")}
          aria-label={t("plan.review.different_side")}
          className={button}
          onClick={() => onDifferentSide(cell)}
        >
          <Shuffle className="size-3.5" strokeWidth={1.5} aria-hidden />
        </button>
      )}
      {onReplace && (
        <button
          type="button"
          tabIndex={-1}
          title={t("plan.review.replace")}
          aria-label={t("plan.review.replace")}
          className={button}
          onClick={() => onReplace(cell)}
        >
          <RefreshCw className="size-3.5" strokeWidth={1.5} aria-hidden />
        </button>
      )}
    </div>
  )
}

function CellContent({
  cell,
  readOnly,
}: {
  cell: BoardCell
  readOnly: boolean
}) {
  const { t } = useTranslation()

  if (cell.plate) {
    return (
      <>
        {cell.badge && <BatchBadge {...cell.badge} />}
        <span className="line-clamp-2 text-small font-semibold text-ink">
          {cell.mainName ?? t("plan.cell.empty_plate")}
        </span>
        {cell.sideText && (
          <span className="line-clamp-1 text-caption text-ink-muted">
            {cell.sideText}
          </span>
        )}
        {cell.subline && (
          <span className="mt-auto text-caption text-ink-muted">
            {t(`plan.cell.subline_${cell.subline}`)}
          </span>
        )}
      </>
    )
  }

  if (cell.mode === "none") {
    return (
      <>
        <span className="text-small text-ink-muted italic">
          {t("plan.cell.no_plate")}
        </span>
        {cell.note && (
          <span className="line-clamp-2 font-heading text-small text-ink-muted italic">
            {cell.note}
          </span>
        )}
      </>
    )
  }

  if (cell.mode === "routine") {
    return (
      <span className="text-caption text-ink-muted">
        {cell.presetName
          ? t("plan.cell.routine_named", { name: cell.presetName })
          : t("plan.cell.routine")}
      </span>
    )
  }

  if (readOnly) return null

  return (
    <span className="m-auto flex items-center gap-1 text-small text-ink-muted">
      <Plus className="size-4" strokeWidth={1.5} aria-hidden="true" />
      {t("plan.cell.add")}
    </span>
  )
}
