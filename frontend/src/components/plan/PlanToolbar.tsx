import { Link } from "@tanstack/react-router"
import {
  CalendarRange,
  ChevronLeft,
  ChevronRight,
  Eraser,
  Keyboard,
  MessageSquare,
  MoreHorizontal,
  ShoppingBasket,
  TriangleAlert,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { GeneratePlanButton } from "@/components/shell/GeneratePlanButton"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { PlanStats, PlanWindow } from "@/lib/api/plan"
import { cn } from "@/lib/utils"

interface PlanToolbarProps {
  window: PlanWindow
  stats: PlanStats
  /** "31 Oct – 6 Nov". */
  rangeLabel: string
  /** The board shows the anchored window (no `start` in the URL). */
  anchored: boolean
  readOnly: boolean
  /** Cells that need attention; the count steps through them. */
  warningCount: number
  onNextWarning: () => void
  onAsk: () => void
  onCopyWeek: () => void
  onClear: () => void
  onShortcuts: () => void
}

const NAV_LINK =
  "inline-flex min-h-11 items-center justify-center rounded-lg text-small text-ink transition-colors duration-150 ease-out hover:bg-panel-sunken md:pointer-fine:min-h-9"

/**
 * The board header (UX §5.5): the range and its stats, window navigation
 * (previous, "This week", next), and the actions: Ask assistant, Shop and an
 * overflow menu with Copy a previous week, Clear this week and the keyboard
 * shortcuts. Past windows lose the editing actions.
 */
export function PlanToolbar({
  window,
  stats,
  rangeLabel,
  anchored,
  readOnly,
  warningCount,
  onNextWarning,
  onAsk,
  onCopyWeek,
  onClear,
  onShortcuts,
}: PlanToolbarProps) {
  const { t } = useTranslation()

  return (
    <header className="flex flex-col gap-3">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="flex min-w-0 flex-col gap-1">
          <h1 className="text-h1 text-ink">{rangeLabel}</h1>
          <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small text-ink-muted tabular-nums">
            <span>
              {t("plan.toolbar.plates", { count: stats.plates })},{" "}
              {t("plan.toolbar.cook_days", { count: stats.cook_days })}
            </span>
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
          </p>
          {readOnly && (
            <p className="text-small text-ink-muted">
              {t("plan.toolbar.read_only")}
            </p>
          )}
        </div>

        <div className="flex items-center gap-2">
          {/* Phone only: the rail carries Generate from 768 px up (D31). */}
          <div className="md:hidden">
            <GeneratePlanButton variant="compact" />
          </div>
          <Button
            variant="outline"
            onClick={onAsk}
            className="h-11 gap-2 px-3 md:pointer-fine:h-9"
          >
            <MessageSquare strokeWidth={1.5} aria-hidden="true" />
            {t("plan.toolbar.ask")}
          </Button>
          <Button
            asChild
            variant="outline"
            className="h-11 gap-2 px-3 md:pointer-fine:h-9"
          >
            <Link to="/shop" search={{ from: window.start, to: window.end }}>
              <ShoppingBasket strokeWidth={1.5} aria-hidden="true" />
              {t("plan.toolbar.shop")}
            </Link>
          </Button>
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
          search={{ start: window.prev_start }}
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
          search={{}}
          aria-current={anchored ? "page" : undefined}
          className={cn(
            NAV_LINK,
            "rounded-full border border-line px-3",
            anchored && "border-accent bg-accent/10 font-semibold text-accent"
          )}
        >
          {t("plan.toolbar.this_week")}
        </Link>
        <Link
          to="/plan"
          search={{ start: window.next_start }}
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
    </header>
  )
}
