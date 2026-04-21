import { useTranslation } from "react-i18next"

import { AnimatedNumber } from "@/components/editorial/AnimatedNumber"
import { cn } from "@/lib/utils"
import { MacroDot } from "./MacroDot"
import { formatGrams, type MacroKind } from "./tokens"

export type MacroChipSize = "xs" | "sm" | "md" | "lg"

interface MacroChipProps {
  kind: MacroKind
  grams: number | null | undefined
  /** Grams for percentage share. Omit to hide the % badge. */
  totalGrams?: number
  size?: MacroChipSize
  /** Render just the abbreviation (P/C/F) in place of the full label. */
  abbreviated?: boolean
  className?: string
}

const TYPO: Record<
  MacroChipSize,
  {
    label: string
    value: string
    gap: string
    pad: string
    dot: "xs" | "sm" | "md"
  }
> = {
  xs: {
    label: "text-[10px] tracking-[0.14em] uppercase",
    value: "text-[12px] font-semibold",
    gap: "gap-1",
    pad: "",
    dot: "xs",
  },
  sm: {
    label: "text-[10px] tracking-[0.18em] uppercase",
    value: "text-sm font-semibold",
    gap: "gap-1.5",
    pad: "",
    dot: "xs",
  },
  md: {
    label: "text-[11px] tracking-[0.18em] uppercase",
    value: "font-heading text-lg font-bold",
    gap: "gap-1.5",
    pad: "rounded-xl bg-surface-container px-3 py-2",
    dot: "sm",
  },
  lg: {
    label: "text-[11px] tracking-[0.2em] uppercase",
    value: "font-heading text-2xl font-bold",
    gap: "gap-2",
    pad: "rounded-2xl bg-surface-container px-4 py-3",
    dot: "sm",
  },
}

export function MacroChip({
  kind,
  grams,
  totalGrams,
  size = "sm",
  abbreviated,
  className,
}: MacroChipProps) {
  const { t } = useTranslation()
  const typo = TYPO[size]
  const label = abbreviated ? t(`macro.${kind}_abbr`) : t(`macro.${kind}`)

  const pct =
    totalGrams != null && totalGrams > 0 && grams != null
      ? Math.round((grams / totalGrams) * 100)
      : null

  const isTile = size === "md" || size === "lg"

  if (isTile) {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center gap-1",
          typo.pad,
          className
        )}
        data-testid={`macro-chip-${kind}`}
      >
        <MacroDot kind={kind} size={typo.dot} />
        <span
          className={cn(
            "block w-full text-center break-words hyphens-auto text-on-surface-variant",
            typo.label
          )}
        >
          {label}
        </span>
        <p className={cn("text-on-surface tabular-nums", typo.value)}>
          {grams == null ? (
            "—"
          ) : (
            <AnimatedNumber value={grams} format={(n) => formatGrams(n)} />
          )}
          <span className="ml-1 text-xs font-medium text-on-surface-variant">
            g
          </span>
          {pct != null && (
            <span className="ml-1.5 text-[10px] font-medium tracking-wider text-on-surface-variant/70 uppercase">
              {pct}%
            </span>
          )}
        </p>
      </div>
    )
  }

  return (
    <span
      className={cn(
        "inline-flex items-center tabular-nums",
        typo.gap,
        className
      )}
      data-testid={`macro-chip-${kind}`}
    >
      <MacroDot kind={kind} size={typo.dot} />
      <span className={cn("text-on-surface-variant", typo.label)}>{label}</span>
      <span className={cn("text-on-surface", typo.value)}>
        {grams == null ? (
          "—"
        ) : (
          <AnimatedNumber value={grams} format={(n) => formatGrams(n)} />
        )}
        <span className="ml-0.5 font-normal text-on-surface-variant">g</span>
      </span>
    </span>
  )
}
