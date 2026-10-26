import { useDraggable, useDroppable, type DragStartEvent } from "@dnd-kit/core"
import type { CSSProperties, ReactNode } from "react"

import type { AvailabilityMode } from "@/lib/api/availability"
import type { Plate } from "@/lib/api/plates"
import { cn } from "@/lib/utils"

interface DndCellWrapperProps {
  day: number
  date?: string
  slotId: number
  /** Slot row index (0-based). Used for grid arrow-key navigation via the
   *  data-cell-pos attribute below. */
  rowIndex: number
  plate: Plate | undefined
  /** The cell's resolved availability mode; `none` cells reject drops. */
  mode?: AvailabilityMode
  children: ReactNode
}

/** What a droppable cell exposes to the drag-end handler. */
export interface DropCellData {
  day: number
  date?: string
  slotId: number
  mode?: AvailabilityMode
  existingPlateId?: number
}

/**
 * The i18n key of the toast for a drop the planner refuses before calling
 * the API (D10), or null when the drop may go ahead. A cell holds one plate
 * and a No plate cell holds none; there is no swap.
 */
export function dropRejectionKey(
  over: Pick<DropCellData, "mode" | "existingPlateId">,
  draggedPlateId: number
): string | null {
  if (over.mode === "none") return "planner.dnd.reject_no_plate"
  if (over.existingPlateId != null && over.existingPlateId !== draggedPlateId) {
    return "planner.dnd.reject_occupied"
  }
  return null
}

// Each planner grid cell is BOTH a droppable target and (if it carries a
// plate) a draggable source. Empty + No plate cells are droppable only.
//
// The wrapper itself is the activator: dnd-kit listeners spread onto this
// div so a pointerdown anywhere on the cell starts a drag once the
// PointerSensor's distance threshold is met (configured in PlannerGrid).
// We deliberately omit `attributes` — those add role="button"/tabIndex=0/
// aria-roledescription, which would nest a button inside the stretched-link
// "open sheet" button living below. Dropping `attributes` also drops
// KeyboardSensor activation; arrow-nav + Enter/N/Del cover the keyboard
// story for the grid.
export function DndCellWrapper({
  day,
  date,
  slotId,
  rowIndex,
  plate,
  mode,
  children,
}: DndCellWrapperProps) {
  const droppableId = `slot:${day}:${slotId}`
  const {
    setNodeRef: setDropRef,
    isOver,
    active,
  } = useDroppable({
    id: droppableId,
    data: {
      day,
      date,
      slotId,
      mode,
      existingPlateId: plate?.id,
    } satisfies DropCellData,
  })

  const draggable = !!plate
  const {
    setNodeRef: setDragRef,
    listeners,
    transform,
    isDragging,
  } = useDraggable({
    id: draggable ? `plate:${plate.id}` : `plate:noop-${day}-${slotId}`,
    data: { plateId: plate?.id, day, date, slotId },
    disabled: !draggable,
  })

  const dragStyle: CSSProperties = transform
    ? {
        transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
      }
    : {}

  // closestCorners always resolves to the nearest droppable, so a No plate or
  // occupied cell would silently snap drops to a neighbour if we disabled it.
  // Keep it active and reject in onDragEnd; render a destructive outline for
  // hover feedback.
  const draggedPlateId = (
    active?.data.current as { plateId?: number } | undefined
  )?.plateId
  const isRejectedDrop =
    isOver &&
    draggedPlateId != null &&
    dropRejectionKey({ mode, existingPlateId: plate?.id }, draggedPlateId) !==
      null

  return (
    <div
      ref={(el) => {
        setDropRef(el)
        if (draggable) setDragRef(el)
      }}
      {...(draggable ? listeners : {})}
      data-testid={`cell-${day}-${slotId}`}
      data-slot-drop-zone={`${day}:${slotId}`}
      data-slot-drag-handle={draggable ? plate.id : undefined}
      data-cell-pos={`${rowIndex}-${day}`}
      data-availability={mode}
      className={cn(
        "relative outline-offset-2",
        isOver &&
          !isRejectedDrop &&
          "rounded-[14px] outline outline-2 outline-primary",
        isRejectedDrop &&
          "rounded-[14px] outline outline-2 outline-destructive",
        isDragging && "opacity-40"
      )}
      style={dragStyle}
    >
      {children}
    </div>
  )
}

export interface DragPayload {
  plateId: number
  fromDay: number
  fromSlotId: number
  mode: "move" | "copy"
}

export function dragStartToPayload(
  event: DragStartEvent,
  copyHeld: boolean
): DragPayload | null {
  const data = event.active.data.current as
    | { plateId?: number; day?: number; slotId?: number }
    | undefined
  if (
    !data ||
    data.plateId === undefined ||
    data.day === undefined ||
    data.slotId === undefined
  ) {
    return null
  }
  return {
    plateId: data.plateId,
    fromDay: data.day,
    fromSlotId: data.slotId,
    mode: copyHeld ? "copy" : "move",
  }
}
