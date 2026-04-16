import type { MacrosResponse } from "@/lib/api/weeks"

const DAY_LABELS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]

interface NutritionDayBarProps {
  day: number
  macros: MacrosResponse
}

export function NutritionDayBar({ day, macros }: NutritionDayBarProps) {
  const total = macros.protein + macros.fat + macros.carbs
  const pct = (v: number) =>
    total > 0 ? `${((v / total) * 100).toFixed(1)}%` : "0%"

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{DAY_LABELS[day]}</span>
        <span>{Math.round(macros.kcal)} kcal</span>
      </div>
      <div
        className="flex h-2 overflow-hidden rounded-full bg-muted"
        role="img"
        aria-label={`${DAY_LABELS[day]}: ${Math.round(macros.kcal)} kcal`}
      >
        <div
          className="bg-[var(--chart-protein)]"
          style={{ width: pct(macros.protein) }}
          title={`Protein ${macros.protein.toFixed(0)}g`}
        />
        <div
          className="bg-[var(--chart-fat)]"
          style={{ width: pct(macros.fat) }}
          title={`Fat ${macros.fat.toFixed(0)}g`}
        />
        <div
          className="bg-[var(--chart-carbs)]"
          style={{ width: pct(macros.carbs) }}
          title={`Carbs ${macros.carbs.toFixed(0)}g`}
        />
      </div>
      <div className="flex gap-3 text-xs text-muted-foreground">
        <span>P {macros.protein.toFixed(0)}g</span>
        <span>F {macros.fat.toFixed(0)}g</span>
        <span>C {macros.carbs.toFixed(0)}g</span>
      </div>
    </div>
  )
}
