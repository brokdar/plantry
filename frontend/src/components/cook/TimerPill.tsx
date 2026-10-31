import { BellRing, Timer, X } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { formatRemaining } from "@/lib/cook/timers"
import { cn } from "@/lib/utils"

export interface RunningTimer {
  id: number
  /** The duration that started it: step index and match offset, e.g.
   *  "2:31". One timer per key at a time. */
  key: string
  /** The step and its wording, e.g. "Step 3 · 20 Minuten". */
  label: string
  /** Epoch ms when it rings. */
  endsAt: number
}

/** Vibrates and plays a short two-tone chime where the browser can. */
function ring() {
  if ("vibrate" in navigator) navigator.vibrate([300, 150, 300])
  const Ctx =
    window.AudioContext ??
    (window as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
  if (!Ctx) return
  try {
    const ctx = new Ctx()
    const gain = ctx.createGain()
    gain.connect(ctx.destination)
    gain.gain.setValueAtTime(0.25, ctx.currentTime)
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 1.2)
    ;[880, 660].forEach((hz, i) => {
      const osc = ctx.createOscillator()
      osc.frequency.value = hz
      osc.connect(gain)
      osc.start(ctx.currentTime + i * 0.3)
      osc.stop(ctx.currentTime + i * 0.3 + 0.6)
    })
    setTimeout(() => void ctx.close(), 1500)
  } catch {
    // No audio output (autoplay policy, no device): the pill still shows it.
  }
}

/**
 * A running step timer (UX §5.3): a pill with its label and the time left
 * that stays in the cook header across steps. At zero it rings once,
 * turns into a "done" pill, and stays until dismissed.
 */
export function TimerPill({
  timer,
  onDismiss,
}: {
  timer: RunningTimer
  onDismiss: () => void
}) {
  const { t } = useTranslation()
  const [now, setNow] = useState(() => Date.now())
  const remaining = timer.endsAt - now
  const done = remaining <= 0

  useEffect(() => {
    if (done) return
    const id = setInterval(() => setNow(Date.now()), 250)
    return () => clearInterval(id)
  }, [done])

  useEffect(() => {
    if (done) ring()
  }, [done])

  return (
    <div
      data-testid="cook-timer"
      data-done={done || undefined}
      className={cn(
        "inline-flex h-11 shrink-0 items-center gap-2 rounded-full border pl-3 text-body",
        done
          ? "border-success bg-panel font-semibold text-success"
          : "border-line bg-panel text-ink"
      )}
    >
      {done ? (
        <BellRing className="size-5" strokeWidth={1.5} aria-hidden />
      ) : (
        <Timer className="size-5" strokeWidth={1.5} aria-hidden />
      )}
      <span>{timer.label}</span>
      <span role="timer" className="tabular-nums">
        {done ? t("cook.timer.done") : formatRemaining(remaining)}
      </span>
      <span role="status" className="sr-only">
        {done ? t("cook.timer.finished", { label: timer.label }) : ""}
      </span>
      <button
        type="button"
        onClick={onDismiss}
        aria-label={t("cook.timer.stop", { label: timer.label })}
        className="inline-flex size-11 items-center justify-center rounded-full text-ink-muted hover:text-ink"
      >
        <X className="size-5" strokeWidth={1.5} aria-hidden />
      </button>
    </div>
  )
}
