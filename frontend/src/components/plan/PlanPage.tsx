import { useNavigate } from "@tanstack/react-router"
import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { BatchHighlightProvider } from "@/components/batch/BatchHighlightProvider"
import { CookStrip } from "@/components/batch/CookStrip"
import { ChatPanel } from "@/components/chat/ChatPanel"
import { CopyFromWeekDialog } from "@/components/presets/CopyFromWeekDialog"
import { Button } from "@/components/ui/button"
import type { Plan } from "@/lib/api/plan"
import { buildBoard, type BoardCell } from "@/lib/plan/board"
import { isCheatsheetShortcut } from "@/lib/plan/keynav"
import { useClearWindow, useMarkNoPlate, usePlan } from "@/lib/queries/plan"
import { useChatUI } from "@/lib/stores/chat-ui"

import { Board } from "./Board"
import { BoardSkeleton } from "./BoardSkeleton"
import { EmptyWindow } from "./EmptyWindow"
import { PlanSheet, type SheetState } from "./PlanSheet"
import { PlanToolbar } from "./PlanToolbar"
import { ShortcutsDialog } from "./ShortcutsDialog"

interface PlanPageProps {
  /** The window start from the URL; undefined opens the anchored window. */
  start?: string
  /** A cell to open its sheet for (`date|slotId`), from `?cell=`. */
  cell?: string
  /** A day to focus (its first cell), from `?day=`. */
  day?: string
}

/**
 * `/plan`: the plan board for one window (UX §5.5, D3). One query
 * (`GET /api/plan`) feeds the toolbar, the cook strip and the board.
 */
export function PlanPage({ start, cell, day }: PlanPageProps) {
  const { t } = useTranslation()
  const plan = usePlan(start)

  if (plan.isPending) {
    return (
      <PageFrame>
        <BoardSkeleton />
      </PageFrame>
    )
  }
  if (plan.isError) {
    return (
      <PageFrame>
        <div role="alert" className="flex flex-col items-start gap-3">
          <p className="text-body text-ink">{t("plan.board.error")}</p>
          <Button variant="outline" onClick={() => void plan.refetch()}>
            {t("common.retry")}
          </Button>
        </div>
      </PageFrame>
    )
  }
  return (
    <PlanView
      plan={plan.data}
      anchored={!start}
      initialCell={cell}
      initialDay={day}
    />
  )
}

function PageFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto flex w-full max-w-[96rem] flex-col gap-4 px-4 py-6 md:px-8 md:py-8">
      {children}
    </div>
  )
}

function parseDate(iso: string) {
  const [y, m, d] = iso.split("-").map(Number)
  return new Date(y, m - 1, d)
}

function PlanView({
  plan,
  anchored,
  initialCell,
  initialDay,
}: {
  plan: Plan
  anchored: boolean
  initialCell?: string
  initialDay?: string
}) {
  const { t, i18n } = useTranslation()
  const board = useMemo(() => buildBoard(plan, i18n), [plan, i18n])
  const boardRef = useRef<HTMLDivElement>(null)
  const [copyOpen, setCopyOpen] = useState(false)
  const [shortcutsOpen, setShortcutsOpen] = useState(false)
  const [warningIndex, setWarningIndex] = useState(0)
  const [sheet, setSheet] = useState<SheetState | null>(
    initialCell ? { key: initialCell } : null
  )
  // The cell the sheet was opened for: focus returns to it on close.
  const sheetCell = useRef<string | null>(initialCell ?? null)
  const navigate = useNavigate()
  const openChat = useChatUI((s) => s.setOpen)
  const clearWindow = useClearWindow()
  const markNoPlate = useMarkNoPlate()
  const { window: win } = plan

  const rangeLabel = new Intl.DateTimeFormat(i18n.language, {
    day: "numeric",
    month: "short",
  }).formatRange(parseDate(win.start), parseDate(win.end))

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (!isCheatsheetShortcut(e)) return
      e.preventDefault()
      setShortcutsOpen(true)
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])

  // Today's week strip opens the board at a day: focus its first cell.
  useEffect(() => {
    if (!initialDay) return
    const el = boardRef.current?.querySelector<HTMLElement>(
      `[data-cell-key^="${initialDay}|"] [data-cell-focus]`
    )
    el?.scrollIntoView({ block: "nearest", inline: "nearest" })
    el?.focus()
  }, [initialDay])

  function focusCell(selector: string) {
    const el = boardRef.current?.querySelector<HTMLElement>(
      `${selector} [data-cell-focus]`
    )
    el?.scrollIntoView({ block: "nearest", inline: "nearest" })
    el?.focus()
  }

  function nextWarning() {
    const keys = board.warningCells
    if (keys.length === 0) return
    const i = warningIndex % keys.length
    focusCell(`[data-cell-key="${keys[i]}"]`)
    setWarningIndex(i + 1)
  }

  /** Opens, switches or closes the sheet. A cell before this window (the
   *  cook cell of a leftover's batch) opens in the previous window. */
  function changeSheet(next: SheetState | null) {
    if (next && next.key.split("|")[0] < win.start) {
      void navigate({
        to: "/plan",
        search: { start: win.prev_start, cell: next.key },
      })
      return
    }
    if (next) sheetCell.current = next.key
    else if (initialCell) {
      // Drop `?cell=` so a reload does not reopen the sheet.
      void navigate({
        to: "/plan",
        search: { start: anchored ? undefined : win.start },
        replace: true,
      })
    }
    setSheet(next)
  }

  function returnFocus() {
    if (sheetCell.current) focusCell(`[data-cell-key="${sheetCell.current}"]`)
  }

  function openCell(cell: BoardCell) {
    // A past window has nothing to do in a cell without a plate.
    if (board.readOnly && !cell.plate) return
    changeSheet({ key: cell.key })
  }

  function markCell(cell: BoardCell) {
    const raw = plan.cells.find(
      (c) => c.date === cell.date && c.slot_id === cell.slotId
    )
    if (raw) markNoPlate.mutate(raw)
  }

  return (
    <PageFrame>
      <PlanToolbar
        window={win}
        stats={plan.stats}
        rangeLabel={rangeLabel}
        anchored={anchored}
        readOnly={board.readOnly}
        warningCount={board.warningCells.length}
        onNextWarning={nextWarning}
        onAsk={() => openChat(true)}
        onCopyWeek={() => setCopyOpen(true)}
        onClear={() => clearWindow.mutate({ from: win.start, to: win.end })}
        onShortcuts={() => setShortcutsOpen(true)}
      />
      <BatchHighlightProvider>
        {board.empty && !board.readOnly && (
          <EmptyWindow
            onPlanByHand={() =>
              focusCell('[data-cell-mode="plan"][data-cell-empty]')
            }
            onCopyWeek={() => setCopyOpen(true)}
          />
        )}
        <CookStrip batches={board.cookStrip} />
        <div ref={boardRef}>
          <Board
            board={board}
            targets={plan.targets}
            label={t("plan.board.label", { range: rangeLabel })}
            onOpenCell={openCell}
            onMarkNoPlate={markCell}
          />
        </div>
      </BatchHighlightProvider>
      <CopyFromWeekDialog
        open={copyOpen}
        onOpenChange={setCopyOpen}
        targetStart={win.start}
        defaultSourceStart={win.prev_start}
      />
      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />
      <PlanSheet
        plan={plan}
        board={board}
        state={sheet}
        onChange={changeSheet}
        onClosed={returnFocus}
      />
      <ChatPanel range={{ from: win.start, to: win.end }} />
    </PageFrame>
  )
}
