import { useDraggable, useDroppable, type DragStartEvent } from "@dnd-kit/core"
import type { CSSProperties, ReactNode } from "react"

import type { Plate } from "@/lib/api/plates"
import { cn } from "@/lib/utils"

interface DndCellWrapperProps {
  day: number
  date?: string
  slotId: number
  plate: Plate | undefined
  children: ReactNode
}

// Each planner grid cell is BOTH a droppable target and (if it carries a
// plate) a draggable source. Empty + skipped cells are droppable only.
// The SlotCell ancestor remains responsible for its content; this wrapper is
// purely a dnd-kit adapter so planner layout stays readable.
export function DndCellWrapper({
  day,
  date,
  slotId,
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

  const draggableId = plate ? `plate:${plate.id}` : null
  const {
    setNodeRef: setDragRef,
    listeners,
    attributes,
    transform,
    isDragging,
  } = useDraggable({
    id: draggableId ?? `plate:noop-${day}-${slotId}`,
    data: { plateId: plate?.id, day, date, slotId },
    disabled: !plate || plate.skipped,
  })

  const dragStyle: CSSProperties = transform
    ? {
        transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
      }
    : {}

  // isOver + active together distinguish hover feedback. We only flag the
  // drop target during an actual drag. The "rejected" outline for skipped
  // cells is rendered here so the user sees it immediately; the final
  // reject-and-toast still happens in onDragEnd.
  const isRejectedDrop = isOver && !!active && plate?.skipped === true

  return (
    <div
      ref={(el) => {
        setDropRef(el)
        if (plate && !plate.skipped) setDragRef(el)
      }}
      {...(plate && !plate.skipped ? listeners : {})}
      {...(plate && !plate.skipped ? attributes : {})}
      data-testid={`cell-${day}-${slotId}`}
      data-slot-drop-zone={`${day}:${slotId}`}
      data-slot-drag-handle={plate && !plate.skipped ? plate.id : undefined}
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
