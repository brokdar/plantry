/** A duration found in a recipe step ("20 Minuten", "10–12 min"). */
export interface DetectedTimer {
  /** Offsets of the match in the step text. */
  start: number
  end: number
  /** The matched text, used as the timer's label. */
  text: string
  /** The duration; a range counts its upper bound. */
  seconds: number
}

export type StepPart =
  | { kind: "text"; text: string }
  | ({ kind: "timer" } & DetectedTimer)

const GLYPHS: Record<string, number> = {
  "¼": 0.25,
  "½": 0.5,
  "¾": 0.75,
  "⅓": 1 / 3,
  "⅔": 2 / 3,
}

const NUM = String.raw`(?:\d+(?:[.,]\d+)?(?:\s?[¼½¾⅓⅔])?|[¼½¾⅓⅔])`
const RANGE = String.raw`\s*(?:[-–—]|to|bis)\s*`
// Longest spellings first; an abbreviation's dot is left out of the match.
const HOURS = "stunden|stunde|std|hours|hour|hrs|hr|h"
const MINUTES = "minutes|minuten|minute|mins|min"

const TIMER = new RegExp(
  String.raw`(?<![\p{L}\d.,])(${NUM})(?:${RANGE}(${NUM}))?\s?(?:(${HOURS})|(${MINUTES}))(?![\p{L}\d])`,
  "giu"
)

function parseNumber(raw: string): number {
  const glyph = raw.match(/[¼½¾⅓⅔]/)?.[0]
  const digits = raw.replace(/[¼½¾⅓⅔\s]/g, "").replace(",", ".")
  return (digits ? Number(digits) : 0) + (glyph ? GLYPHS[glyph] : 0)
}

/**
 * The durations in a step, in order: minutes and hours in English and
 * German, with decimals ("1,5 Std"), fractions ("1½ hours") and ranges
 * ("10–12 min", "10 bis 15 Minuten"), which count their upper bound.
 */
export function detectTimers(text: string): DetectedTimer[] {
  const timers: DetectedTimer[] = []
  for (const m of text.matchAll(TIMER)) {
    const value = parseNumber(m[2] ?? m[1])
    const seconds = Math.round(value * (m[3] ? 3600 : 60))
    if (seconds <= 0) continue
    timers.push({
      start: m.index,
      end: m.index + m[0].length,
      text: m[0],
      seconds,
    })
  }
  return timers
}

/** A step's text split around its timers, for rendering timer buttons
 *  inline. */
export function splitByTimers(text: string): StepPart[] {
  const parts: StepPart[] = []
  let at = 0
  for (const timer of detectTimers(text)) {
    if (timer.start > at) {
      parts.push({ kind: "text", text: text.slice(at, timer.start) })
    }
    parts.push({ kind: "timer", ...timer })
    at = timer.end
  }
  if (at < text.length) parts.push({ kind: "text", text: text.slice(at) })
  return parts
}

/** "4:05", "20:00", "1:30:00"; never negative. Rounds up, so a timer
 *  reads 0:00 only once it is done. */
export function formatRemaining(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = String(total % 60).padStart(2, "0")
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`
}
