import { ChevronLeft, ChevronRight, Timer } from "lucide-react"
import { useEffect, useRef, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { splitByTimers, type DetectedTimer } from "@/lib/cook/timers"

/** A horizontal swipe longer than this turns the page. */
const SWIPE_PX = 48

interface StepPagerProps {
  /** Step texts in order. */
  steps: readonly string[]
  /** A duration button in step `step` (0-based) was pressed. */
  onStartTimer: (timer: DetectedTimer, step: number) => void
  /** Shown under the last step (or alone without steps): "Done cooking". */
  finish: ReactNode
  /** Shown on the last step only, e.g. "Freeze 2 portions". */
  lastStepNote?: ReactNode
}

function isTyping(target: EventTarget | null) {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName) ||
      target.closest('[role="tablist"]') !== null)
  )
}

/**
 * One recipe step per screen (UX §5.3) in 22 px Newsreader, with durations
 * as timer buttons. Previous and Next are 56 px targets at the bottom; the
 * arrow keys and a horizontal swipe turn the page too. The last step
 * swaps Next for `finish`.
 */
export function StepPager({
  steps,
  onStartTimer,
  finish,
  lastStepNote,
}: StepPagerProps) {
  const { t } = useTranslation()
  const [index, setIndex] = useState(0)
  const touch = useRef<{ x: number; y: number } | null>(null)
  const total = steps.length
  const last = index >= total - 1

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.altKey || e.ctrlKey || e.metaKey || isTyping(e.target)) return
      if (e.key === "ArrowRight") setIndex((i) => Math.min(total - 1, i + 1))
      else if (e.key === "ArrowLeft") setIndex((i) => Math.max(0, i - 1))
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [total])

  if (total === 0) {
    return (
      <div className="flex flex-col gap-6">
        <p className="text-h3 text-ink-muted">{t("cook.no_steps")}</p>
        {finish}
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col gap-6" data-testid="step-pager">
      <section
        aria-roledescription={t("cook.step_role")}
        aria-label={t("cook.step_of", { step: index + 1, total })}
        className="flex flex-1 touch-pan-y flex-col gap-4"
        onTouchStart={(e) => {
          const p = e.touches[0]
          touch.current = { x: p.clientX, y: p.clientY }
        }}
        onTouchEnd={(e) => {
          const start = touch.current
          touch.current = null
          if (!start) return
          const p = e.changedTouches[0]
          const dx = p.clientX - start.x
          if (
            Math.abs(dx) < SWIPE_PX ||
            Math.abs(dx) < Math.abs(p.clientY - start.y)
          )
            return
          setIndex((i) =>
            dx < 0 ? Math.min(total - 1, i + 1) : Math.max(0, i - 1)
          )
        }}
      >
        <p
          className="text-h3 text-ink-muted tabular-nums"
          aria-live="polite"
          data-testid="step-position"
        >
          {t("cook.step_of", { step: index + 1, total })}
        </p>
        <p
          className="max-w-[68ch] font-heading text-[1.375rem] leading-relaxed text-ink"
          data-testid="step-text"
        >
          {splitByTimers(steps[index]).map((part, i) =>
            part.kind === "text" ? (
              <span key={i}>{part.text}</span>
            ) : (
              <button
                key={i}
                type="button"
                onClick={() => onStartTimer(part, index)}
                aria-label={t("cook.timer.start", { label: part.text })}
                className="mx-0.5 inline-flex min-h-11 items-center gap-1.5 rounded-lg border border-accent px-2 align-middle font-sans text-h3 font-semibold text-accent tabular-nums hover:bg-accent hover:text-on-accent"
              >
                <Timer className="size-5" strokeWidth={1.5} aria-hidden />
                {part.text}
              </button>
            )
          )}
        </p>
        {last && lastStepNote}
      </section>
      <div className="flex flex-col gap-3">
        {last && finish}
        <div className="grid grid-cols-2 gap-3">
          <Button
            type="button"
            variant="outline"
            className="h-14 gap-2 text-h3"
            disabled={index === 0}
            onClick={() => setIndex((i) => Math.max(0, i - 1))}
          >
            <ChevronLeft className="size-5" strokeWidth={1.5} aria-hidden />
            {t("cook.previous")}
          </Button>
          <Button
            type="button"
            variant={last ? "outline" : "default"}
            className="h-14 gap-2 text-h3"
            disabled={last}
            onClick={() => setIndex((i) => Math.min(total - 1, i + 1))}
          >
            {t("cook.next")}
            <ChevronRight className="size-5" strokeWidth={1.5} aria-hidden />
          </Button>
        </div>
      </div>
    </div>
  )
}
