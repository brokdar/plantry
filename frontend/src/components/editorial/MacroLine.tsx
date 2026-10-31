import { useTranslation } from "react-i18next"

import { formatNumber } from "@/lib/plan/format"
import { cn } from "@/lib/utils"

type Value = number | null | undefined

interface MacroLineProps {
  kcal: Value
  protein: Value
  fat: Value
  carbs: Value
  className?: string
}

/** kcal, protein, fat and carbs as one line of text: "452 kcal · P 31 g ·
 *  F 12 g · C 50 g". kcal in ink, the rest muted; no per-macro colours. */
export function MacroLine({
  kcal,
  protein,
  fat,
  carbs,
  className,
}: MacroLineProps) {
  const { t, i18n } = useTranslation()
  const num = (v: Value, digits = 1) =>
    v == null ? "—" : formatNumber(v, i18n, digits)
  const g = t("units.g")
  const macros: [string, Value][] = [
    [t("macro.protein_abbr"), protein],
    [t("macro.fat_abbr"), fat],
    [t("macro.carbs_abbr"), carbs],
  ]
  return (
    <p
      className={cn("text-small text-ink-muted tabular-nums", className)}
      data-testid="nutrition-line"
    >
      <span className="font-semibold text-ink">
        {num(kcal, 0)} {t("units.kcal")}
      </span>
      {macros.map(([label, value]) => (
        <span key={label}>
          {" · "}
          {label} {num(value)} {g}
        </span>
      ))}
    </p>
  )
}
