import {
  DndContext,
  MouseSensor,
  pointerWithin,
  TouchSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core"
import { useRef, useSyncExternalStore } from "react"
import { useTranslation } from "react-i18next"

import type { BoardCell } from "@/lib/plan/board"
import { copySnapshot } from "@/lib/plan/board"
import { useRestorePlan } from "@/lib/queries/plan"
import { useUpdatePlate } from "@/lib/queries/plates"
import { toast } from "@/lib/toast"

import { BoardGrid, type BoardLayoutProps } from "./BoardGrid"
import { DayList } from "./DayList"

const PHONE_QUERY = "(max-width: 767px)"

function subscribe(onChange: () => void) {
  if (typeof window.matchMedia !== "function") return () => {}
  const mq = window.matchMedia(PHONE_QUERY)
  mq.addEventListener("change", onChange)
  return () => mq.removeEventListener("change", onChange)
}

function isPhone() {
  return (
    typeof window.matchMedia === "function" &&
    window.matchMedia(PHONE_QUERY).matches
  )
}

function cellOf(data: unknown): BoardCell | undefined {
  return (data as { cell?: BoardCell } | undefined)?.cell
}

/**
 * The board: the day list on phones, the grid from 768 px, both from the
 * same view model (UX §9 "one board, three layouts"). Owns drag and drop:
 * a drag moves the plate; holding Ctrl/⌘ when the drag starts copies it as
 * a fresh meal through restore. Drops onto No plate or occupied cells are
 * refused here, before any request.
 */
export function Board(props: BoardLayoutProps) {
  const { t } = useTranslation()
  const phone = useSyncExternalStore(subscribe, isPhone)
  const updatePlate = useUpdatePlate()
  const restore = useRestorePlan()
  const copy = useRef(false)

  const sensors = useSensors(
    // A short move separates a drag from a click that opens the cell.
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    // Touch (tablet) needs a press-and-hold so swiping still scrolls.
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    })
  )

  function onDragStart(e: DragStartEvent) {
    const a = e.activatorEvent as MouseEvent | TouchEvent | null
    copy.current = !!a && (a.ctrlKey || a.metaKey)
  }

  function onDragEnd(e: DragEndEvent) {
    const from = cellOf(e.active.data.current)
    const to = cellOf(e.over?.data.current)
    if (!from?.plate || !to || to.key === from.key) return
    if (to.mode === "none") {
      toast.error(t("planner.dnd.reject_no_plate"))
      return
    }
    if (to.plate) {
      toast.error(t("planner.dnd.reject_occupied"))
      return
    }
    if (copy.current) {
      restore.mutate(copySnapshot(from.plate, to.date, to.slotId))
    } else {
      updatePlate.mutate({
        id: from.plate.id,
        input: { date: to.date, slot_id: to.slotId },
      })
    }
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={pointerWithin}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
    >
      {phone ? <DayList {...props} /> : <BoardGrid {...props} />}
    </DndContext>
  )
}
