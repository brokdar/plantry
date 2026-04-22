import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { MACRO_DOT_CLASS, MACRO_KCAL_PER_G } from "./tokens"

type MacroGrams = {
  protein: number | null | undefined
  carbs: number | null | undefined
  fat: number | null | undefined
}

type Targets = { protein?: number; carbs?: number; fat?: number; kcal?: number }

interface MacroDistributionBarProps {
  values: MacroGrams
  /**
   * `kcal` (default): segments sized by kcal contribution of each macro.
   * `grams`: segments sized by raw grams.
   * `targets`: segments render as progress toward each target (capped at 100%).
   */
  mode?: "kcal" | "grams" | "targets"
  targets?: Targets
  thickness?: "xs" | "sm" | "md" | "lg"
  track?: "surface-container-highest" | "surface-container" | "muted"
  className?: string
  label?: string
}

const THICKNESS: Record<
  NonNullable<MacroDistributionBarProps["thickness"]>,
  string
> = {
  xs: "h-1",
  sm: "h-1.5",
  md: "h-2",
  lg: "h-2.5",
}

const TRACK: Record<NonNullable<MacroDistributionBarProps["track"]>, string> = {
  "surface-container-highest": "bg-surface-container-highest",
  "surface-container": "bg-surface-container",
  muted: "bg-muted",
}

type TriKind = "protein" | "carbs" | "fat"
const ORDER: TriKind[] = ["protein", "carbs", "fat"]

export function MacroDistributionBar({
  values,
  mode = "kcal",
  targets,
  thickness = "sm",
  track = "surface-container-highest",
  className,
  label,
}: MacroDistributionBarProps) {
  const { t } = useTranslation()

  const shares = ORDER.map((kind) => {
    const grams = values[kind] ?? 0
    if (mode === "grams") return { kind, value: grams }
    if (mode === "kcal") return { kind, value: grams * MACRO_KCAL_PER_G[kind] }
    const target = (targets?.[kind] ?? 0) as number
    if (target <= 0) return { kind, value: 0 }
    return { kind, value: Math.min(grams / target, 1) }
  })

  const total =
    mode === "targets"
      ? 3 // each macro bucket caps at 1 → three lanes
      : Math.max(
          shares.reduce((s, x) => s + x.value, 0),
          1
        )

  return (
    <div
      role="img"
      aria-label={label ?? t("macro.distribution")}
      className={cn(
        "flex w-full overflow-hidden rounded-full",
        TRACK[track],
        THICKNESS[thickness],
        className
      )}
      data-testid="macro-distribution-bar"
    >
      {shares.map(({ kind, value }) => {
        const pct = Math.max(0, Math.min(100, (value / total) * 100))
        if (pct <= 0) return null
        return (
          <div
            key={kind}
            className={cn("h-full", MACRO_DOT_CLASS[kind])}
            style={{ width: `${pct}%` }}
            data-macro={kind}
            aria-hidden
          />
        )
      })}
    </div>
  )
}
