import { useTranslation } from "react-i18next"

import { MacroDot } from "@/components/editorial/macros"
import type { MacrosResponse } from "@/lib/api/weeks"

interface SlotMacroDotsProps {
  macros?: MacrosResponse
}

export function SlotMacroDots({ macros }: SlotMacroDotsProps) {
  const { t } = useTranslation()
  if (!macros) return null
  return (
    <div className="flex items-center justify-between gap-2 font-mono text-[10.5px] text-on-surface-variant tabular-nums">
      <span className="font-heading text-[11.5px] font-bold tracking-tight text-on-surface">
        {Math.round(macros.kcal)} {t("macro.kcal")}
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
