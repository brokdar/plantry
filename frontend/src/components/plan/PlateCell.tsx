import { useDraggable, useDroppable } from "@dnd-kit/core"
import { Lock, Plus, RefreshCw, Shuffle, TriangleAlert } from "lucide-react"
import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import {
  useBatchCell,
  useBatchHighlight,
} from "@/components/batch/batch-highlight"
import { imageURL } from "@/lib/image-url"
import { cellInChange } from "@/lib/assistant/pulse"
import type { BoardCell } from "@/lib/plan/board"
import { BATCH_CLASSES, BATCH_HIGHLIGHT_CLASS } from "@/lib/plan/batch-colours"
import { useLastChange } from "@/lib/stores/assistant-stream"
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

/** Visual state of the cell's surface: a hairline border and a whisper of
 *  shadow. Draft plates are dashed, a cook cell takes its batch tint, a
 *  violation turns the border warning-coloured, No plate is a transparent
 *  dashed cell. */
function surfaceClass(cell: BoardCell): string {
  const draft = cell.state === "draft"
  if (cell.plate) {
    return cn(
      cell.badge?.variant === "cook"
        ? BATCH_CLASSES[cell.badge.colour].tint
        : "bg-panel",
      cell.attention
        ? "border-[1.5px] border-dashed border-warning"
        : draft
          ? "border border-dashed border-ink-muted/50"
          : "border border-line shadow-card"
    )
  }
  if (cell.mode === "none")
    return "border border-dashed border-line bg-transparent"
  return "border border-line bg-panel shadow-card"
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
  const { highlight } = useBatchHighlight()
  // Pointing at or focusing a card brings its batch forward.
  const pointAtBatch = () => {
    if (cell.badge) highlight(cell.badge.sessionId, cell.badge.colour)
  }
  // The cell the assistant's last change touched pulses once (D17).
  const pulse = cellInChange(cell, useLastChange())
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
      data-pulse={pulse ? "" : undefined}
      onPointerEnter={pointAtBatch}
      onFocus={pointAtBatch}
      className={cn(
        "group/cell @container/cell relative min-w-0 rounded-cell transition-opacity duration-200 ease-out focus-within:z-20",
        // While a batch is highlighted the other cells step back.
        "[[data-highlight-active]_&:not([data-highlighted])]:opacity-50",
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
          "flex w-full min-w-0 flex-col items-stretch gap-1.5 rounded-cell p-2 text-left transition-colors duration-150 ease-out @[7rem]/cell:p-2.5",
          grid ? "h-full min-h-[5.25rem]" : "min-h-14",
          surfaceClass(cell),
          !cell.plate &&
            cell.mode !== "none" &&
            !readOnly &&
            "hover:bg-panel-sunken",
          cell.mode === "none" &&
            "items-center justify-center hover:bg-panel-sunken",
          isDragging && "relative z-30 opacity-80",
          isOver && !isDragging && "border-accent bg-accent/10"
        )}
      >
        <CellContent cell={cell} readOnly={readOnly} />
      </button>
      {cell.attention && (
        <TriangleAlert
          data-testid="cell-warning"
          className="pointer-events-none absolute top-2 right-2 size-3.5 text-warning"
          strokeWidth={2}
          aria-hidden="true"
        />
      )}
      {cell.state === "confirmed" && (
        <Lock
          data-testid="cell-padlock"
          className={cn(
            "pointer-events-none absolute top-2 size-3 text-ink-muted",
            cell.attention ? "right-7" : "right-2"
          )}
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
 * Something else and Change side on hover (fine pointers only: touch reaches
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
      className="absolute right-1.5 bottom-1.5 hidden gap-1 opacity-0 transition-opacity duration-150 ease-out group-focus-within/cell:opacity-100 group-hover/cell:opacity-100 pointer-fine:flex"
    >
      {onDifferentSide && cell.sideComponentId !== null && (
        <button
          type="button"
          tabIndex={-1}
          title={t("plan.alt.change_side")}
          aria-label={t("plan.alt.change_side")}
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
          title={t("plan.alt.something_else")}
          aria-label={t("plan.alt.something_else")}
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
    const name = cell.mainName ?? t("plan.cell.empty_plate")
    return (
      <>
        <span
          className={cn(
            "flex min-w-0 items-start gap-2",
            cell.state === "confirmed" && "pr-4"
          )}
        >
          <Thumb name={name} image={cell.mainImage} />
          <span className="line-clamp-2 min-w-0 text-small leading-tight font-semibold text-ink">
            {name}
          </span>
        </span>
        <span className="mt-auto flex min-w-0 items-center gap-1.5">
          {cell.badge && <BatchBadge {...cell.badge} inCell />}
          {cell.sideText ? (
            <span className="min-w-0 truncate text-caption text-ink-muted">
              {cell.sideText}
            </span>
          ) : (
            cell.subline && (
              <span
                className={cn(
                  "min-w-0 truncate text-caption text-ink-muted",
                  // The badge already says it; screen readers still hear it.
                  cell.badge && "sr-only"
                )}
              >
                {t(`plan.cell.subline_${cell.subline}`)}
              </span>
            )
          )}
        </span>
      </>
    )
  }

  if (cell.mode === "none") {
    return (
      <>
        <span className="text-small text-ink-muted">
          {t("plan.cell.no_plate")}
        </span>
        {cell.note && (
          <span className="line-clamp-2 text-caption text-ink-muted">
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

const THUMB_TINTS = [
  "bg-batch-1-tint",
  "bg-batch-2-tint",
  "bg-batch-3-tint",
  "bg-batch-4-tint",
  "bg-batch-5-tint",
  "bg-batch-6-tint",
]

/** The main food's 30 px thumbnail; without an image, a tinted square with
 *  the food's initial (the tint follows the name, so a food keeps its own). */
function Thumb({ name, image }: { name: string; image: string | null }) {
  const src = imageURL(image)
  if (src) {
    return (
      <img
        src={src}
        alt=""
        loading="lazy"
        className="hidden size-[30px] shrink-0 rounded-lg border border-line object-cover @[7rem]/cell:block"
      />
    )
  }
  let hash = 0
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) >>> 0
  return (
    <span
      aria-hidden="true"
      className={cn(
        "hidden size-[30px] shrink-0 place-items-center rounded-lg text-small font-bold text-ink-muted @[7rem]/cell:grid",
        THUMB_TINTS[hash % THUMB_TINTS.length]
      )}
    >
      {name.trim().charAt(0).toUpperCase()}
    </span>
  )
}
