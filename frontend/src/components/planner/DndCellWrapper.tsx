import {
  useDraggable,
  useDroppable,
  type DragStartEvent,
  type DraggableAttributes,
  type DraggableSyntheticListeners,
} from "@dnd-kit/core"
import type { CSSProperties } from "react"

import type { Plate } from "@/lib/api/plates"
import { cn } from "@/lib/utils"

/**
 * Drag handle exposed to the cell content. When present, the inner element
 * that should initiate drag (the stretched-link button) attaches the ref +
 * listeners + attributes. This keeps the outer wrapper free of role="button"
 * / aria-roledescription / tabIndex — those a11y annotations live on the
 * actual interactive element instead, so the cell's accessibility tree stays
 * flat (one button per cell, not nested).
 */
export interface DragHandle {
  setActivatorNodeRef: (element: HTMLElement | null) => void
  listeners: DraggableSyntheticListeners
  attributes: DraggableAttributes
}

interface DndCellWrapperProps {
  day: number
  date?: string
  slotId: number
  /** Slot row index (0-based). Used for grid arrow-key navigation via the
   *  data-cell-pos attribute below. */
  rowIndex: number
  plate: Plate | undefined
  /**
   * Render-prop. Receives a DragHandle when the cell is draggable
   * (planned + not skipped); otherwise null. Children attach the handle to
   * the element that should initiate drag.
   */
  children: (handle: DragHandle | null) => React.ReactNode
}

// Each planner grid cell is BOTH a droppable target and (if it carries a
// plate) a draggable source. Empty + skipped cells are droppable only.
export function DndCellWrapper({
  day,
  date,
  slotId,
  rowIndex,
  plate,
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
      existingPlateId: plate?.id,
      skipped: plate?.skipped,
    },
  })

  const draggable = plate && !plate.skipped
  const {
    setNodeRef: setDragRef,
    setActivatorNodeRef,
    listeners,
    attributes,
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

  // closestCorners always resolves to the nearest droppable, so a skipped cell
  // would silently snap drops to a neighbour if we disabled it. Keep it active
  // and reject in onDragEnd; render a destructive outline for hover feedback.
  const isRejectedDrop = isOver && !!active && plate?.skipped === true

  const handle: DragHandle | null = draggable
    ? { setActivatorNodeRef, listeners, attributes }
    : null

  return (
    <div
      ref={(el) => {
        setDropRef(el)
        if (draggable) setDragRef(el)
      }}
      data-testid={`cell-${day}-${slotId}`}
      data-slot-drop-zone={`${day}:${slotId}`}
      data-slot-drag-handle={draggable ? plate.id : undefined}
      data-cell-pos={`${rowIndex}-${day}`}
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
      {children(handle)}
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
