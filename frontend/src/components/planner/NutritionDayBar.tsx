import type { MacrosResponse } from "@/lib/api/weeks"

const DAY_LABELS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]

export interface NutritionTargets {
  kcal: number
  proteinG: number
  fatG: number
  carbsG: number
}

interface NutritionDayBarProps {
  day: number
  macros: MacrosResponse
  targets?: NutritionTargets
}

export function NutritionDayBar({
  day,
  macros,
  targets,
}: NutritionDayBarProps) {
  const hasTargets = !!targets && targets.kcal > 0

  // Without targets: relative macro distribution; with targets: progress toward target
  const barWidth = (actual: number, target: number) => {
    if (!hasTargets || target <= 0) return "0%"
    return `${Math.min((actual / target) * 100, 100).toFixed(1)}%`
  }

  const pctOfTotal = (v: number) => {
    const total = macros.protein + macros.fat + macros.carbs
    return total > 0 ? `${((v / total) * 100).toFixed(1)}%` : "0%"
  }

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{DAY_LABELS[day]}</span>
        <span>
          {Math.round(macros.kcal)} kcal
          {hasTargets && (
            <span className="ml-1 text-muted-foreground/70">
              / {targets!.kcal}
            </span>
          )}
        </span>
      </div>
      <div
        className="flex h-2 overflow-hidden rounded-full bg-muted"
        role="img"
        aria-label={`${DAY_LABELS[day]}: ${Math.round(macros.kcal)} kcal`}
      >
        {hasTargets ? (
          <>
            <div
              className="bg-[var(--chart-protein)]"
              style={{ width: barWidth(macros.protein, targets!.proteinG) }}
              title={`Protein ${macros.protein.toFixed(0)}g / ${targets!.proteinG.toFixed(0)}g`}
            />
            <div
              className="bg-[var(--chart-fat)]"
              style={{ width: barWidth(macros.fat, targets!.fatG) }}
              title={`Fat ${macros.fat.toFixed(0)}g / ${targets!.fatG.toFixed(0)}g`}
            />
            <div
              className="bg-[var(--chart-carbs)]"
              style={{ width: barWidth(macros.carbs, targets!.carbsG) }}
              title={`Carbs ${macros.carbs.toFixed(0)}g / ${targets!.carbsG.toFixed(0)}g`}
            />
          </>
        ) : (
          <>
            <div
              className="bg-[var(--chart-protein)]"
              style={{ width: pctOfTotal(macros.protein) }}
              title={`Protein ${macros.protein.toFixed(0)}g`}
            />
            <div
              className="bg-[var(--chart-fat)]"
              style={{ width: pctOfTotal(macros.fat) }}
              title={`Fat ${macros.fat.toFixed(0)}g`}
            />
            <div
              className="bg-[var(--chart-carbs)]"
              style={{ width: pctOfTotal(macros.carbs) }}
              title={`Carbs ${macros.carbs.toFixed(0)}g`}
            />
          </>
        )}
      </div>
      <div className="flex gap-3 text-xs text-muted-foreground">
        <span>P {macros.protein.toFixed(0)}g</span>
        <span>F {macros.fat.toFixed(0)}g</span>
        <span>C {macros.carbs.toFixed(0)}g</span>
      </div>
    </div>
  )
}
