import {
  BookmarkPlus,
  ChevronLeft,
  GripVertical,
  Trash2,
  Utensils,
  type LucideIcon,
} from "lucide-react"
import { useRef, useState } from "react"
import { createPortal } from "react-dom"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

/**
 * Single touch-gesture surface that wraps one mobile slot card. Handles
 * three discrete gestures from the same pointer stream so they never collide
 * with each other or with native vertical scroll:
 *
 *   • tap        → forwarded to inner stretched-link button (opens sheet)
 *   • swipe-left → reveals action drawer (Skip / Save / Delete)
 *   • long-press → enters drag mode; parent does day-tab hit-testing and
 *                  emits the move on drop
 *
 * Decision rule (touch only — mouse falls through to inner clicks):
 *   - vertical drift > 14 px before any decision → release control,
 *     letting the page scroll
 *   - horizontal drift > 8 px (and ≥ vertical) → swipe wins
 *   - 380 ms held within tolerance → drag wins
 *
 * `touch-action: pan-y` blocks the browser's horizontal pan so swipe stays
 * with us, while vertical scroll still passes through.
 */

const DRAWER_WIDTH = 168
const SWIPE_THRESHOLD = 8
const VERTICAL_LOCKOUT = 14
const LONG_PRESS_MS = 380
const SNAP_OPEN_FRACTION = 0.4

type State = "idle" | "swipe" | "drag"

interface MobileSlotRowProps {
  planned: boolean
  drawerOpen: boolean
  setDrawerOpen: (open: boolean) => void
  onSkip: () => void
  onSave?: () => void
  onDelete: () => void
  /** Called when long-press elapses and drag mode begins. */
  onDragStart?: () => void
  /** Called on every move while in drag mode. Parent hit-tests day tabs. */
  onDragMove?: (clientX: number, clientY: number) => void
  /** Called when the drag pointer releases. Parent commits the move. */
  onDragEnd?: (clientX: number, clientY: number) => void
  /** Short label rendered inside the drag ghost. */
  dragLabel: string
  testId?: string
  children: React.ReactNode
}

export function MobileSlotRow({
  planned,
  drawerOpen,
  setDrawerOpen,
  onSkip,
  onSave,
  onDelete,
  onDragStart,
  onDragMove,
  onDragEnd,
  dragLabel,
  testId,
  children,
}: MobileSlotRowProps) {
  const { t } = useTranslation()
  // Live offset while the user's finger is on the screen. `null` means
  // "not actively swiping", so the rendered translate is derived from the
  // parent-owned `drawerOpen` prop. This single source of truth keeps the
  // visual in sync with external opens/closes without a useEffect.
  const [swipeOffset, setSwipeOffset] = useState<number | null>(null)
  const [phase, setPhase] = useState<State>("idle")
  const [dragPos, setDragPos] = useState<{ x: number; y: number } | null>(null)

  const restingTranslate = planned && drawerOpen ? -DRAWER_WIDTH : 0
  const translateX = swipeOffset ?? restingTranslate
  const animate = swipeOffset === null

  // All gesture decisions live in a ref so back-to-back pointer events (which
  // run synchronously in one task) read the latest phase. Reading React state
  // here would return the stale render-time value and miss the swipe → drag
  // transition.
  const gesture = useRef({
    pointerId: -1,
    startX: 0,
    startY: 0,
    baseTranslate: 0,
    currentOffset: 0,
    decided: false,
    phase: "idle" as State,
    timer: 0 as number | null,
  })

  function setPhaseBoth(next: State) {
    gesture.current.phase = next
    setPhase(next)
  }

  function clearTimer() {
    if (gesture.current.timer != null) {
      window.clearTimeout(gesture.current.timer)
      gesture.current.timer = null
    }
  }

  function handlePointerDown(e: React.PointerEvent) {
    if (!planned) return
    if (e.pointerType === "mouse") return
    gesture.current.pointerId = e.pointerId
    gesture.current.startX = e.clientX
    gesture.current.startY = e.clientY
    gesture.current.baseTranslate = restingTranslate
    gesture.current.decided = false
    clearTimer()
    if (!drawerOpen) {
      gesture.current.timer = window.setTimeout(() => {
        if (gesture.current.decided) return
        gesture.current.decided = true
        setPhaseBoth("drag")
        setDragPos({ x: gesture.current.startX, y: gesture.current.startY })
        onDragStart?.()
        if ("vibrate" in navigator) navigator.vibrate?.(15)
      }, LONG_PRESS_MS)
    }
  }

  function handlePointerMove(e: React.PointerEvent) {
    if (!planned || e.pointerId !== gesture.current.pointerId) return
    const dx = e.clientX - gesture.current.startX
    const dy = e.clientY - gesture.current.startY

    if (gesture.current.phase === "drag") {
      setDragPos({ x: e.clientX, y: e.clientY })
      onDragMove?.(e.clientX, e.clientY)
      return
    }

    if (!gesture.current.decided) {
      if (Math.abs(dy) > VERTICAL_LOCKOUT && Math.abs(dy) > Math.abs(dx)) {
        clearTimer()
        gesture.current.decided = true
        gesture.current.pointerId = -1
        return
      }
      if (Math.abs(dx) > SWIPE_THRESHOLD && Math.abs(dx) >= Math.abs(dy)) {
        clearTimer()
        gesture.current.decided = true
        setPhaseBoth("swipe")
      }
    }

    if (gesture.current.phase === "swipe") {
      const next = Math.max(
        -DRAWER_WIDTH,
        Math.min(0, gesture.current.baseTranslate + dx)
      )
      gesture.current.currentOffset = next
      setSwipeOffset(next)
    }
  }

  function handlePointerUp(e: React.PointerEvent) {
    if (e.pointerId !== gesture.current.pointerId) return
    clearTimer()
    const endingPhase = gesture.current.phase
    if (endingPhase === "drag") {
      onDragEnd?.(e.clientX, e.clientY)
      setDragPos(null)
      setPhaseBoth("idle")
    } else if (endingPhase === "swipe") {
      const offset = gesture.current.currentOffset
      const opened = offset < -DRAWER_WIDTH * SNAP_OPEN_FRACTION
      setSwipeOffset(null)
      setDrawerOpen(opened)
      setPhaseBoth("idle")
    }
    gesture.current.pointerId = -1
  }

  function handlePointerCancel(e: React.PointerEvent) {
    if (e.pointerId !== gesture.current.pointerId) return
    clearTimer()
    if (gesture.current.phase === "drag") {
      setDragPos(null)
      setPhaseBoth("idle")
    } else if (gesture.current.phase === "swipe") {
      setSwipeOffset(null)
      setPhaseBoth("idle")
    }
    gesture.current.pointerId = -1
  }

  return (
    <div
      className="relative"
      data-testid={testId}
      aria-label={planned ? t("planner.mobile.row_actions") : undefined}
    >
      {planned && (
        <div
          className="absolute inset-y-0 right-0 flex items-stretch overflow-hidden rounded-[14px]"
          style={{ width: DRAWER_WIDTH }}
          aria-hidden={!drawerOpen}
        >
          <DrawerButton
            onClick={() => {
              onSkip()
              setDrawerOpen(false)
            }}
            tone="tertiary"
            shortLabel={t("planner.mobile.skip_short")}
            ariaLabel={t("skip.mark")}
            Icon={Utensils}
            tabIndex={drawerOpen ? 0 : -1}
            testId="mobile-row-skip"
          />
          {onSave && (
            <DrawerButton
              onClick={() => {
                onSave()
                setDrawerOpen(false)
              }}
              tone="primary"
              shortLabel={t("planner.mobile.save_short")}
              ariaLabel={t("template.save_as")}
              Icon={BookmarkPlus}
              tabIndex={drawerOpen ? 0 : -1}
              testId="mobile-row-save"
            />
          )}
          <DrawerButton
            onClick={() => {
              onDelete()
              setDrawerOpen(false)
            }}
            tone="destructive"
            shortLabel={t("planner.mobile.delete_short")}
            ariaLabel={t("plate.delete_plate")}
            Icon={Trash2}
            tabIndex={drawerOpen ? 0 : -1}
            testId="mobile-row-delete"
          />
        </div>
      )}

      <div
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerCancel}
        data-mobile-row-state={phase}
        data-drawer-open={drawerOpen ? "true" : undefined}
        className={cn(
          "relative",
          phase === "drag" && "opacity-40",
          animate && "transition-transform duration-200 ease-out",
          planned && "touch-pan-y select-none"
        )}
        style={{ transform: `translate3d(${translateX}px, 0, 0)` }}
      >
        {/* React 19 supports `inert` natively; covering the SlotCell while
            the drawer is exposed pulls its inner stretched-link button out of
            the focus order so keyboard users land on the drawer actions
            (skip / save / delete) instead of the hidden card behind them. */}
        <div inert={drawerOpen}>{children}</div>

        {/* Persistent grip glyph on planned rows: gives the swipe + long-
            press affordance an explicit visual + a screen-reader handle.
            Pointer events pass through (the gesture handler lives on the
            parent), so this is purely a visual cue. */}
        {planned && !drawerOpen && (
          <div
            className="pointer-events-none absolute top-1/2 right-1.5 z-[2] flex -translate-y-1/2 flex-col items-center gap-0.5 rounded-full bg-surface-container-lowest/85 px-1 py-1.5 text-on-surface-variant/60 opacity-90 shadow-sm transition-opacity"
            aria-hidden
            data-testid="mobile-row-grip"
          >
            <ChevronLeft className="h-3 w-3" aria-hidden />
            <GripVertical className="h-3 w-3" aria-hidden />
          </div>
        )}
        {/* Off-screen text node carrying the gesture description for AT.
            Lives next to the gesture surface so VoiceOver / TalkBack
            announces the affordance when a user lands on the row. */}
        {planned && (
          <span className="sr-only" data-testid="mobile-row-gesture-hint">
            {t("planner.mobile.swipe_hint")} ·{" "}
            {t("planner.mobile.drag_handle_label")}
          </span>
        )}

        {drawerOpen && (
          <button
            type="button"
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              setDrawerOpen(false)
            }}
            aria-label={t("common.close")}
            data-testid="mobile-row-close-drawer"
            className="absolute inset-0 z-10 rounded-[14px]"
          />
        )}
      </div>

      {phase === "drag" &&
        dragPos &&
        createPortal(
          <DragGhost x={dragPos.x} y={dragPos.y} label={dragLabel} />,
          document.body
        )}
    </div>
  )
}

function DrawerButton({
  onClick,
  tone,
  shortLabel,
  ariaLabel,
  Icon,
  tabIndex,
  testId,
}: {
  onClick: () => void
  tone: "primary" | "tertiary" | "destructive"
  shortLabel: string
  ariaLabel: string
  Icon: LucideIcon
  tabIndex: number
  testId: string
}) {
  const toneCls = {
    primary: "bg-primary text-on-primary hover:bg-primary/90",
    tertiary: "bg-tertiary text-on-tertiary hover:bg-tertiary/90",
    destructive:
      "bg-destructive text-destructive-foreground hover:bg-destructive/90",
  }[tone]
  return (
    <button
      type="button"
      onClick={onClick}
      tabIndex={tabIndex}
      aria-label={ariaLabel}
      data-testid={testId}
      className={cn(
        "flex flex-1 flex-col items-center justify-center gap-1 px-1 transition-colors active:scale-[0.98]",
        toneCls
      )}
    >
      <Icon className="h-4 w-4" aria-hidden />
      <span className="font-heading text-[10.5px] font-bold tracking-[0.12em] uppercase">
        {shortLabel}
      </span>
    </button>
  )
}

function DragGhost({ x, y, label }: { x: number; y: number; label: string }) {
  const { t } = useTranslation()
  return (
    <div
      className="pointer-events-none fixed z-[60] -translate-x-1/2 -translate-y-1/2"
      style={{ left: x, top: y }}
      aria-hidden
    >
      <div className="flex items-center gap-2 rounded-full border border-primary/40 bg-surface-container-lowest px-3 py-2 shadow-lg">
        <GripVertical className="h-3.5 w-3.5 text-primary" aria-hidden />
        <span className="max-w-[180px] truncate font-heading text-[11px] font-bold tracking-[0.18em] text-on-surface uppercase">
          {label}
        </span>
      </div>
      <div className="mt-1 text-center font-heading text-[9px] font-bold tracking-[0.2em] text-on-surface-variant uppercase">
        {t("planner.mobile.drop_to_move")}
      </div>
    </div>
  )
}
