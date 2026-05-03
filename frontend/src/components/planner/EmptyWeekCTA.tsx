import { Copy, FileDown, Sparkles, X } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"

interface EmptyWeekCTAProps {
  /** Window-from date — used both as the dismissal storage key and to compute
   *  the previous window for "Copy last week". */
  windowFrom: string
  aiEnabled: boolean
  copying: boolean
  onCopyLastWeek: () => void
  onApplyTemplate: () => void
  onAiFill: () => void
}

const DISMISS_PREFIX = "plantry.emptyWeekDismiss."

function readDismissed(windowFrom: string): boolean {
  if (typeof window === "undefined") return false
  try {
    return window.localStorage.getItem(DISMISS_PREFIX + windowFrom) === "1"
  } catch {
    return false
  }
}

function persistDismissed(windowFrom: string) {
  if (typeof window === "undefined") return
  try {
    window.localStorage.setItem(DISMISS_PREFIX + windowFrom, "1")
  } catch {
    // Non-fatal — banner reappears next time, no data lost.
  }
}

export function EmptyWeekCTA({
  windowFrom,
  aiEnabled,
  copying,
  onCopyLastWeek,
  onApplyTemplate,
  onAiFill,
}: EmptyWeekCTAProps) {
  const { t } = useTranslation()
  // Re-derive on window change — the user may dismiss week A then navigate
  // to week B which is also empty, where the banner should appear again.
  const [dismissed, setDismissed] = useState(() => readDismissed(windowFrom))
  useEffect(() => {
    setDismissed(readDismissed(windowFrom))
  }, [windowFrom])

  if (dismissed) return null

  function handleDismiss() {
    persistDismissed(windowFrom)
    setDismissed(true)
  }

  return (
    <section
      data-testid="empty-week-cta"
      aria-labelledby="empty-week-cta-title"
      className="editorial-shadow relative overflow-hidden rounded-2xl border border-outline-variant/40 bg-surface-container-lowest"
    >
      {/* Botanical gradient wash — keeps the banner aesthetically aligned with
          the planner without competing with the grid below. */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 opacity-60"
        style={{
          backgroundImage:
            "radial-gradient(ellipse at top right, color-mix(in oklab, var(--primary) 12%, transparent) 0%, transparent 60%), radial-gradient(ellipse at bottom left, color-mix(in oklab, var(--secondary) 10%, transparent) 0%, transparent 55%)",
        }}
      />
      <button
        type="button"
        onClick={handleDismiss}
        aria-label={t("planner.empty_week.dismiss")}
        data-testid="empty-week-dismiss"
        className="absolute top-3 right-3 z-[1] grid size-7 place-items-center rounded-full text-on-surface-variant/70 transition-colors hover:bg-surface-container hover:text-on-surface"
      >
        <X className="h-3.5 w-3.5" aria-hidden />
      </button>
      <div className="relative flex flex-col gap-5 px-6 py-7 md:px-8 md:py-9">
        <div className="space-y-1.5">
          <p className="font-heading text-[10px] font-bold tracking-[0.22em] text-primary uppercase">
            {t("planner.empty_week.eyebrow")}
          </p>
          <h2
            id="empty-week-cta-title"
            className="font-heading text-2xl font-bold text-on-surface md:text-[28px]"
          >
            {t("planner.empty_week.title")}
          </h2>
          <p className="max-w-xl text-sm text-on-surface-variant">
            {t("planner.empty_week.body")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            onClick={onCopyLastWeek}
            disabled={copying}
            data-testid="empty-week-copy"
            className="gradient-primary editorial-shadow border-0 text-on-primary hover:opacity-90"
          >
            <Copy className="mr-1.5 h-3.5 w-3.5" aria-hidden />
            {copying
              ? t("planner.empty_week.copying")
              : t("planner.empty_week.copy_last_week")}
          </Button>
          <Button
            variant="outline"
            onClick={onApplyTemplate}
            data-testid="empty-week-apply-template"
          >
            <FileDown className="mr-1.5 h-3.5 w-3.5" aria-hidden />
            {t("planner.empty_week.apply_template")}
          </Button>
          {aiEnabled && (
            <Button
              variant="outline"
              onClick={onAiFill}
              data-testid="empty-week-ai-fill"
            >
              <Sparkles className="mr-1.5 h-3.5 w-3.5" aria-hidden />
              {t("planner.empty_week.ai_fill")}
            </Button>
          )}
          <Button
            variant="ghost"
            onClick={handleDismiss}
            data-testid="empty-week-fresh"
            className="text-on-surface-variant"
          >
            {t("planner.empty_week.start_fresh")}
          </Button>
        </div>
      </div>
    </section>
  )
}
