import { useNavigate } from "@tanstack/react-router"
import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { BatchHighlightProvider } from "@/components/batch/BatchHighlightProvider"
import { CookStrip } from "@/components/batch/CookStrip"
import { ChatPanel } from "@/components/chat/ChatPanel"
import { CopyFromWeekDialog } from "@/components/plan/CopyFromWeekDialog"
import { Button } from "@/components/ui/button"
import type { Plan, ReplanCell } from "@/lib/api/plan"
import { buildBoard, type BoardCell } from "@/lib/plan/board"
import { isAcceptShortcut, isCheatsheetShortcut } from "@/lib/plan/keynav"
import { replanFrom } from "@/lib/plan/replan"
import { useGenerate, useRegenerate, useRun } from "@/lib/queries/generation"
import {
  useAccept,
  useClearWindow,
  useDiscard,
  useMarkNoPlate,
  usePlan,
} from "@/lib/queries/plan"

import { Board } from "./Board"
import { BoardSkeleton } from "./BoardSkeleton"
import { DiscardDialog } from "./DiscardDialog"
import { DraftBar } from "./DraftBar"
import { EmptyWindow } from "./EmptyWindow"
import { GenerationProgress } from "./GenerationProgress"
import { PlanSheet, type SheetState } from "./PlanSheet"
import { PlanToolbar } from "./PlanToolbar"
import { ReplanSheet } from "./ReplanSheet"
import { ShortcutsDialog } from "./ShortcutsDialog"

interface PlanPageProps {
  /** The window start from the URL; undefined opens the anchored window. */
  start?: string
  /** A cell to open its sheet for (`date|slotId`), from `?cell=`. */
  cell?: string
  /** A day to focus (its first cell), from `?day=`. */
  day?: string
  /** The generation the board follows (D35), from `?run=`. */
  run?: number
}

/**
 * `/plan`: the plan board for one window (UX §5.5, D3). One query
 * (`GET /api/plan`) feeds the toolbar, the cook strip and the board.
 */
export function PlanPage({ start, cell, day, run }: PlanPageProps) {
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
      run={run}
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
  run,
}: {
  plan: Plan
  anchored: boolean
  initialCell?: string
  initialDay?: string
  run?: number
}) {
  const { t, i18n } = useTranslation()
  const board = useMemo(() => buildBoard(plan, i18n), [plan, i18n])
  const boardRef = useRef<HTMLDivElement>(null)
  const [copyOpen, setCopyOpen] = useState(false)
  const [shortcutsOpen, setShortcutsOpen] = useState(false)
  const [discardOpen, setDiscardOpen] = useState(false)
  // The re-plan sheet, the cells it opened with (UX §5.10) and the board
  // cell it was opened from, which gets focus back when it closes.
  const [replan, setReplan] = useState<{
    open: boolean
    cells: ReplanCell[]
    origin?: string
  }>({ open: false, cells: [] })
  const replanOpen = useRef(false)
  // Needs attention: the next cell to step to, and the cell the last press
  // focused (the next press opens its sheet).
  const [attention, setAttention] = useState<{
    index: number
    focused: string | null
  }>({ index: 0, focused: null })
  const [sheet, setSheet] = useState<SheetState | null>(
    initialCell ? { key: initialCell } : null
  )
  // The cell the sheet was opened for: focus returns to it on close.
  const sheetCell = useRef<string | null>(initialCell ?? null)
  const navigate = useNavigate()
  const [chatOpen, setChatOpen] = useState(false)
  const clearWindow = useClearWindow()
  const markNoPlate = useMarkNoPlate()
  const accept = useAccept()
  const discard = useDiscard()
  const generate = useGenerate()
  const regenerate = useRegenerate()
  const { window: win } = plan

  // The generation the board follows: the one in the URL, else one the
  // server says is running for this window (a reload re-attaches, D35).
  const runId = run ?? plan.running_run_id ?? undefined
  const runQuery = useRun(runId)
  const running =
    runId !== undefined &&
    !runQuery.isError &&
    runQuery.data?.status !== "ok" &&
    runQuery.data?.status !== "error" &&
    runQuery.data?.status !== "partial"
  const canReview = board.draft !== null && !board.readOnly
  // The first day a re-plan may touch; null for a past window.
  const from = replanFrom(win)

  const rangeLabel = new Intl.DateTimeFormat(i18n.language, {
    day: "numeric",
    month: "short",
  }).formatRange(parseDate(win.start), parseDate(win.end))

  function acceptAndShop() {
    if (!canReview || running || accept.isPending) return
    const range = { from: win.start, to: win.end }
    accept.mutate(range, {
      onSuccess: () => void navigate({ to: "/shop", search: range }),
    })
  }

  const onKey = useEffectEvent((e: KeyboardEvent) => {
    if (isCheatsheetShortcut(e)) {
      e.preventDefault()
      setShortcutsOpen(true)
    } else if (isAcceptShortcut(e) && canReview) {
      e.preventDefault()
      acceptAndShop()
    }
  })

  useEffect(() => {
    const listener = (e: KeyboardEvent) => onKey(e)
    window.addEventListener("keydown", listener)
    return () => window.removeEventListener("keydown", listener)
  }, [])

  /** Replace a draft plate (keyboard R, quick action): its batch closure. */
  function replace(cell: BoardCell) {
    if (!cell.plate) return
    regenerate.mutate({
      scope: { kind: "plate", plate_id: cell.plate.id },
      start: win.start,
    })
  }

  /** Different side (keyboard S, quick action): only the side. */
  function differentSide(cell: BoardCell) {
    if (cell.sideComponentId === null) return
    regenerate.mutate({
      scope: { kind: "side", component_id: cell.sideComponentId },
      start: win.start,
    })
  }

  /** Drops `?run=` once the run is over, so a reload doesn't announce it
   *  again. */
  function runFinished() {
    if (run === undefined) return
    void navigate({
      to: "/plan",
      search: { start: anchored ? undefined : win.start },
      replace: true,
    })
  }

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

  /** The attention count steps through the cells: one press focuses the
   *  next cell, the press after it opens that cell's sheet. */
  function nextWarning() {
    const keys = board.warningCells
    if (keys.length === 0) return
    const { index, focused } = attention
    if (focused && keys.includes(focused)) {
      setAttention({ index, focused: null })
      changeSheet({ key: focused })
      return
    }
    const i = index % keys.length
    focusCell(`[data-cell-key="${keys[i]}"]`)
    setAttention({ index: i + 1, focused: keys[i] })
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
    // Mark as No plate moved on to the re-plan sheet: focus stays there.
    if (replanOpen.current) return
    if (sheetCell.current) focusCell(`[data-cell-key="${sheetCell.current}"]`)
  }

  function openCell(cell: BoardCell) {
    // A past window has nothing to do in a cell without a plate.
    if (board.readOnly && !cell.plate) return
    changeSheet({ key: cell.key })
  }

  function openReplan(cells: ReplanCell[], origin?: string) {
    replanOpen.current = true
    setReplan({ open: true, cells, origin })
  }

  /** Mark as No plate: a confirmed plate from today on goes through the
   *  re-plan sheet (its ingredients may be bought, D34); anything else is
   *  marked at once with Undo. */
  function markCell(cell: BoardCell) {
    if (cell.state === "confirmed" && from !== null && cell.date >= from) {
      openReplan([{ date: cell.date, slot_id: cell.slotId }], cell.key)
      return
    }
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
        summary={board.summary}
        rangeLabel={rangeLabel}
        readOnly={board.readOnly}
        warningCount={board.warningCells.length}
        onNextWarning={nextWarning}
        onAsk={() => setChatOpen(true)}
        onCopyWeek={() => setCopyOpen(true)}
        onClear={() => clearWindow.mutate({ from: win.start, to: win.end })}
        onShortcuts={() => setShortcutsOpen(true)}
        draft={board.draft}
        running={running}
        accepting={accept.isPending}
        onAcceptAndShop={acceptAndShop}
        onRegenerateWeek={() => generate.mutate({ start: win.start })}
        onDiscard={() => setDiscardOpen(true)}
        // Re-plan from today: only the current window (D33).
        onReplan={
          win.is_current && !board.readOnly ? () => openReplan([]) : undefined
        }
        progress={
          runId !== undefined && (
            <GenerationProgress
              key={runId}
              run={runQuery.data}
              failed={runQuery.isError}
              attention={board.warningCells.length}
              onFinished={runFinished}
            />
          )
        }
      />
      <BatchHighlightProvider>
        {board.empty && !board.readOnly && !running && (
          <EmptyWindow
            start={win.start}
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
            // While a run goes, R/S and the quick actions wait for it.
            onReplace={running ? undefined : replace}
            onDifferentSide={running ? undefined : differentSide}
          />
        </div>
      </BatchHighlightProvider>
      {canReview && (
        <DraftBar
          summary={board.summary}
          disabled={running || accept.isPending}
          onAccept={acceptAndShop}
        />
      )}
      <DiscardDialog
        open={discardOpen}
        onOpenChange={setDiscardOpen}
        plates={plan.draft?.plates ?? 0}
        pending={discard.isPending}
        onDiscard={() =>
          discard.mutate(
            { from: win.start, to: win.end },
            { onSuccess: () => setDiscardOpen(false) }
          )
        }
      />
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
        running={running}
        onChange={changeSheet}
        onClosed={returnFocus}
        onMarkNoPlate={markCell}
      />
      {from !== null && (
        <ReplanSheet
          open={replan.open}
          onOpenChange={(open) => {
            replanOpen.current = open
            setReplan((r) => ({ ...r, open }))
          }}
          plan={plan}
          from={from}
          initialCells={replan.cells}
          onCloseAutoFocus={(e) => {
            if (!replan.origin) return
            e.preventDefault()
            focusCell(`[data-cell-key="${replan.origin}"]`)
          }}
        />
      )}
      <ChatPanel open={chatOpen} onOpenChange={setChatOpen} start={win.start} />
    </PageFrame>
  )
}
