import { Link } from "@tanstack/react-router"
import {
  CalendarClock,
  CalendarRange,
  Check,
  ChevronLeft,
  ChevronRight,
  Eraser,
  Keyboard,
  MessageSquare,
  MoreHorizontal,
  RefreshCw,
  ShoppingBasket,
  Trash2,
  TriangleAlert,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { GeneratePlanButton } from "@/components/shell/GeneratePlanButton"
import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { PlanStats, PlanWindow } from "@/lib/api/plan"
import { windowStartOf } from "@/lib/dates"
import type { DraftSummary } from "@/lib/plan/board"
import { cn } from "@/lib/utils"

interface PlanToolbarProps {
  window: PlanWindow
  stats: PlanStats
  /** The board's summary line (`Board.summary`). */
  summary: string
  /** "31 Oct – 6 Nov". */
  rangeLabel: string
  readOnly: boolean
  /** Cells that need attention; the count steps through them. */
  warningCount: number
  onNextWarning: () => void
  /** The assistant panel is open (`?assistant=1`); the week links keep it. */
  assistantOpen: boolean
  /** An AI provider is configured; without one the button is disabled. */
  assistantEnabled: boolean
  onToggleAssistant: () => void
  onCopyWeek: () => void
  onClear: () => void
  onShortcuts: () => void
  /** The window's drafts; null when there are none (D33 review actions). */
  draft: DraftSummary | null
  /** A generation runs for this window: review actions wait for it. */
  running: boolean
  accepting: boolean
  onAcceptAndShop: () => void
  onRegenerateWeek: () => void
  onDiscard: () => void
  /** Re-plan from today; only given for the current window. */
  onReplan?: () => void
  /** The progress bar of a running generation (D35). */
  progress?: React.ReactNode
}

const NAV_LINK =
  "inline-flex min-h-11 items-center justify-center rounded-lg text-small text-ink transition-colors duration-150 ease-out hover:bg-panel-sunken md:pointer-fine:min-h-9"

/**
 * The board header (UX §5.5): the range and its stats ("Draft · …" with
 * drafts), the run's notes, window navigation (previous, "This week", next),
 * the generation progress, and the actions: Ask assistant, Shop, Accept and
 * shop (with drafts) and an overflow menu with Regenerate week and Discard
 * draft (with drafts), Re-plan from today (the current window), Copy a
 * previous week, Clear this week and the keyboard shortcuts. Past windows
 * lose the editing actions.
 */
export function PlanToolbar({
  window,
  stats,
  summary,
  rangeLabel,
  readOnly,
  warningCount,
  onNextWarning,
  assistantOpen,
  assistantEnabled,
  onToggleAssistant,
  onCopyWeek,
  onClear,
  onShortcuts,
  draft,
  running,
  accepting,
  onAcceptAndShop,
  onRegenerateWeek,
  onDiscard,
  onReplan,
  progress,
}: PlanToolbarProps) {
  const { t } = useTranslation()
  const review = draft !== null && !readOnly
  const keep = assistantOpen ? (1 as const) : undefined

  return (
    <header className="flex flex-col gap-3">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="flex min-w-0 flex-col gap-1">
          <h1 className="text-h1 text-ink">{rangeLabel}</h1>
          <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small text-ink-muted tabular-nums">
            <span data-testid="plan-summary">{summary}</span>
            {warningCount > 0 && (
              <button
                type="button"
                onClick={onNextWarning}
                data-testid="plan-warnings"
                className="inline-flex min-h-11 items-center gap-1 rounded-lg px-1 font-semibold text-warning hover:underline md:pointer-fine:min-h-6"
              >
                <TriangleAlert
                  className="size-4"
                  strokeWidth={1.5}
                  aria-hidden="true"
                />
                {t("plan.toolbar.warnings", { count: warningCount })}
              </button>
            )}
            {/* Reviewing a draft, "0 plates need attention" is the answer
                the review looks for (AC-1 step 4). */}
            {warningCount === 0 && review && (
              <span data-testid="plan-warnings">
                {t("plan.toolbar.warnings", { count: 0 })}
              </span>
            )}
          </p>
          {readOnly && (
            <p className="text-small text-ink-muted">
              {t("plan.toolbar.read_only")}
            </p>
          )}
          {/* Touch has no hover: the reason shows as text. */}
          {!assistantEnabled && (
            <p
              data-testid="assistant-disabled-reason"
              className="max-w-prose text-small text-ink-muted pointer-fine:hidden"
            >
              {t("assistant.disabled_reason")}
            </p>
          )}
          {draft?.notes && (
            <p
              data-testid="plan-notes"
              className="max-w-prose text-small text-ink"
            >
              {draft.notes}
            </p>
          )}
        </div>

        <div className="flex items-center gap-2">
          {/* Phone only: the rail carries Generate from 768 px up (D31). */}
          <div className="md:hidden">
            <GeneratePlanButton variant="compact" />
          </div>
          {/* Below 640 px Ask and Shop are icons (their names stay for
              screen readers), so the row fits a phone in German too. */}
          <TooltipProvider delayDuration={200}>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  onClick={assistantEnabled ? onToggleAssistant : undefined}
                  aria-disabled={!assistantEnabled || undefined}
                  aria-pressed={assistantEnabled ? assistantOpen : undefined}
                  data-assistant-toggle=""
                  className="h-11 gap-2 px-3 aria-disabled:opacity-50 max-sm:w-11 max-sm:px-0 md:pointer-fine:h-9"
                >
                  <MessageSquare strokeWidth={1.5} aria-hidden="true" />
                  <span className="max-sm:sr-only">
                    {t("plan.toolbar.ask")}
                  </span>
                </Button>
              </TooltipTrigger>
              {!assistantEnabled && (
                <TooltipContent>
                  {t("assistant.disabled_reason")}
                </TooltipContent>
              )}
            </Tooltip>
          </TooltipProvider>
          <Button
            asChild
            variant="outline"
            className="h-11 gap-2 px-3 max-sm:w-11 max-sm:px-0 md:pointer-fine:h-9"
          >
            <Link to="/shop" search={{ from: window.start, to: window.end }}>
              <ShoppingBasket strokeWidth={1.5} aria-hidden="true" />
              <span className="max-sm:sr-only">{t("plan.toolbar.shop")}</span>
            </Link>
          </Button>
          {/* Phones get Accept and shop in the sticky DraftBar. */}
          {review && (
            <Button
              onClick={onAcceptAndShop}
              disabled={running || accepting}
              className="h-11 gap-2 px-3 max-md:hidden md:pointer-fine:h-9"
            >
              <Check strokeWidth={1.5} aria-hidden="true" />
              {t("plan.review.accept_and_shop")}
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                aria-label={t("plan.toolbar.more")}
                className="size-11 md:pointer-fine:size-9"
              >
                <MoreHorizontal strokeWidth={1.5} aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {review && (
                <>
                  <DropdownMenuItem
                    disabled={running}
                    onSelect={onRegenerateWeek}
                  >
                    <RefreshCw strokeWidth={1.5} aria-hidden="true" />
                    {t("plan.review.regenerate_week")}
                  </DropdownMenuItem>
                  <DropdownMenuItem disabled={running} onSelect={onDiscard}>
                    <Trash2 strokeWidth={1.5} aria-hidden="true" />
                    {t("plan.review.discard")}
                  </DropdownMenuItem>
                </>
              )}
              {onReplan && (
                <DropdownMenuItem disabled={running} onSelect={onReplan}>
                  <CalendarClock strokeWidth={1.5} aria-hidden="true" />
                  {t("plan.replan.open")}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem disabled={readOnly} onSelect={onCopyWeek}>
                <CalendarRange strokeWidth={1.5} aria-hidden="true" />
                {t("plan.toolbar.copy_week")}
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={readOnly || stats.plates === 0}
                onSelect={onClear}
              >
                <Eraser strokeWidth={1.5} aria-hidden="true" />
                {t("plan.toolbar.clear")}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={onShortcuts}>
                <Keyboard strokeWidth={1.5} aria-hidden="true" />
                {t("plan.toolbar.shortcuts")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <nav
        aria-label={t("plan.toolbar.weeks")}
        className="flex items-center gap-1"
      >
        <Link
          to="/plan"
          search={{ start: window.prev_start, assistant: keep }}
          aria-label={t("plan.toolbar.prev")}
          className={cn(NAV_LINK, "w-11 md:pointer-fine:w-9")}
        >
          <ChevronLeft
            className="size-5"
            strokeWidth={1.5}
            aria-hidden="true"
          />
        </Link>
        <Link
          to="/plan"
          // The window holding today, as on Today, whatever the anchor.
          search={{
            start: windowStartOf(window.today, window.shopping_day),
            assistant: keep,
          }}
          aria-current={window.is_current ? "page" : undefined}
          className={cn(
            NAV_LINK,
            "rounded-full border border-line px-3",
            window.is_current &&
              "border-accent bg-accent/10 font-semibold text-accent"
          )}
        >
          {t("plan.toolbar.this_week")}
        </Link>
        <Link
          to="/plan"
          search={{ start: window.next_start, assistant: keep }}
          aria-label={t("plan.toolbar.next")}
          className={cn(NAV_LINK, "w-11 md:pointer-fine:w-9")}
        >
          <ChevronRight
            className="size-5"
            strokeWidth={1.5}
            aria-hidden="true"
          />
        </Link>
      </nav>
      {progress}
    </header>
  )
}
