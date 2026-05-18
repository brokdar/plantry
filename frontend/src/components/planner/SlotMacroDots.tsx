import { useTranslation } from "react-i18next"

import { MacroDot } from "@/components/editorial/macros"
import type { MacrosResponse } from "@/lib/api/plates"
import { cn } from "@/lib/utils"

interface SlotMacroDotsProps {
  macros?: MacrosResponse
  /** Per-slot kcal target (daily target ÷ slot count). When provided alongside
   *  `macros`, a coloured ring renders next to the kcal number to show how the
   *  plate compares to its share of the day's budget. */
  kcalTarget?: number | null
}

type Tone = "good" | "near" | "off"

/** Compares actual to target and bands the result the same way the planner
 *  spec calls out: ±10 % is "good" (green), ±20 % is "near" (amber), beyond
 *  is "off" (red). Returns null when no target is set so the caller can hide
 *  the indicator instead of drawing a misleading dot. */
function macrosTone(actual: number, target: number): Tone | null {
  if (target <= 0) return null
  const diff = Math.abs(actual - target) / target
  if (diff <= 0.1) return "good"
  if (diff <= 0.2) return "near"
  return "off"
}

const TONE_COLORS: Record<Tone, string> = {
  good: "bg-primary/85 ring-primary/35",
  near: "bg-ai-accent ring-ai-accent/35",
  off: "bg-destructive/85 ring-destructive/30",
}

const TONE_LABEL: Record<Tone, string> = {
  good: "macro.target.on",
  near: "macro.target.near",
  off: "macro.target.off",
}

export function SlotMacroDots({ macros, kcalTarget }: SlotMacroDotsProps) {
  const { t } = useTranslation()

  // Render a fixed-height placeholder so the cell doesn't reflow when the
  // /plates/macros query resolves on first paint.
  if (!macros) {
    return (
      <div
        className="flex items-center justify-between gap-2 font-mono text-[10.5px] text-on-surface-variant/50 tabular-nums"
        aria-hidden
        data-testid="slot-cell-macros-placeholder"
      >
        <span className="font-heading text-[11.5px] font-bold tracking-tight">
          — {t("macro.kcal")}
        </span>
      </div>
    )
  }

  const tone =
    kcalTarget && kcalTarget > 0 ? macrosTone(macros.kcal, kcalTarget) : null

  return (
    <div className="flex items-center justify-between gap-2 font-mono text-[10.5px] text-on-surface-variant tabular-nums">
      <span className="flex items-center gap-1.5">
        {tone && (
          <span
            data-testid="slot-macros-target-dot"
            data-tone={tone}
            aria-label={t(TONE_LABEL[tone])}
            className={cn("size-1.5 rounded-full ring-2", TONE_COLORS[tone])}
          />
        )}
        <span
          className="font-heading text-[11.5px] font-bold tracking-tight text-on-surface"
          data-testid="slot-cell-kcal"
        >
          {Math.round(macros.kcal)} {t("macro.kcal")}
        </span>
      </span>
      <span className="flex items-center gap-1.5">
        <Chip
          kind="protein"
          value={macros.protein}
          abbr={t("macro.protein_abbr")}
        />
        <Chip kind="carbs" value={macros.carbs} abbr={t("macro.carbs_abbr")} />
        <Chip kind="fat" value={macros.fat} abbr={t("macro.fat_abbr")} />
      </span>
    </div>
  )
}

function Chip({
  kind,
  value,
  abbr,
}: {
  kind: "protein" | "carbs" | "fat"
  value: number
  abbr: string
}) {
  return (
    <span className="flex items-center gap-1">
      <MacroDot kind={kind} size="xs" />
      {abbr}
      {Math.round(value)}
    </span>
  )
}
