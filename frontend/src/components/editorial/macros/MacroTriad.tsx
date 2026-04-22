import { cn } from "@/lib/utils"
import { MacroChip, type MacroChipSize } from "./MacroChip"

type MacroValues = {
  protein: number | null | undefined
  carbs: number | null | undefined
  fat: number | null | undefined
}

interface MacroTriadProps {
  values: MacroValues
  size?: MacroChipSize
  abbreviated?: boolean
  /** Tile sizes render as grid; inline sizes render as flex-wrap row. */
  layout?: "auto" | "grid" | "inline"
  /** Show each chip's % of total macro grams (tile sizes only). */
  showShare?: boolean
  className?: string
}

export function MacroTriad({
  values,
  size = "sm",
  abbreviated,
  layout = "auto",
  showShare,
  className,
}: MacroTriadProps) {
  const isTile = size === "md" || size === "lg"
  const resolved = layout === "auto" ? (isTile ? "grid" : "inline") : layout

  const total =
    showShare && isTile
      ? (values.protein ?? 0) + (values.carbs ?? 0) + (values.fat ?? 0)
      : undefined

  const containerClass =
    resolved === "grid"
      ? "grid grid-cols-3 gap-2"
      : "flex flex-wrap items-center gap-x-4 gap-y-1.5"

  return (
    <div className={cn(containerClass, className)} data-testid="macro-triad">
      <MacroChip
        kind="protein"
        grams={values.protein}
        size={size}
        abbreviated={abbreviated}
        totalGrams={total}
      />
      <MacroChip
        kind="carbs"
        grams={values.carbs}
        size={size}
        abbreviated={abbreviated}
        totalGrams={total}
      />
      <MacroChip
        kind="fat"
        grams={values.fat}
        size={size}
        abbreviated={abbreviated}
        totalGrams={total}
      />
    </div>
  )
}
