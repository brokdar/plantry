import { useTranslation } from "react-i18next"
import { Database, Globe, Sparkles, TriangleAlert } from "lucide-react"

import {
  MacroDistributionBar,
  MacroKcalHero,
  MacroTriad,
} from "@/components/editorial/macros"
import { Badge } from "@/components/ui/badge"
import type { LookupCandidate } from "@/lib/api/lookup"
import { cn } from "@/lib/utils"

interface NutritionDetailProps {
  candidate: LookupCandidate
  /** Show the AI "recommended" marker next to the name. */
  recommended?: boolean
  className?: string
}

type Row = { labelKey: string; value: number | null | undefined; unit: string }

function anyFilled(rows: Row[]) {
  return rows.some((r) => r.value != null)
}

function fmt(value: number, unit: string) {
  // Keep label typography tight: 1 decimal for sub-10 values in g/mg, whole otherwise.
  const precision = value < 10 && unit !== "µg" ? 1 : 0
  // Strip trailing .0 so "3.0 g" → "3 g".
  const text = value.toFixed(precision).replace(/\.0$/, "")
  return `${text} ${unit}`
}

/**
 * NutritionDetail renders the macros + extended nutrient sections for a
 * lookup candidate. Layout is deliberately compact and editorial — inline
 * flex-wrap groups with tight tracking, matching the Botanical Atelier
 * surface-container treatment used elsewhere in the app.
 *
 * Sections without any upstream data are elided so a candidate with only
 * basic macros does not render empty minerals/vitamins bands.
 */
export function NutritionDetail({
  candidate,
  recommended,
  className,
}: NutritionDetailProps) {
  const { t } = useTranslation()

  const hasCoreMacros =
    candidate.kcal_100g != null ||
    candidate.protein_100g != null ||
    candidate.fat_100g != null ||
    candidate.carbs_100g != null

  const extended: Row[] = [
    {
      labelKey: "nutrition.saturated_fat",
      value: candidate.saturated_fat_100g,
      unit: "g",
    },
    {
      labelKey: "nutrition.trans_fat",
      value: candidate.trans_fat_100g,
      unit: "g",
    },
    {
      labelKey: "nutrition.cholesterol",
      value: candidate.cholesterol_100g,
      unit: "mg",
    },
    { labelKey: "ingredient.fiber", value: candidate.fiber_100g, unit: "g" },
    { labelKey: "nutrition.sugar", value: candidate.sugar_100g, unit: "g" },
  ]

  // Sodium is stored in g upstream; food labels show mg, so convert for display.
  const sodiumMg =
    candidate.sodium_100g != null ? candidate.sodium_100g * 1000 : null
  const minerals: Row[] = [
    { labelKey: "ingredient.sodium", value: sodiumMg, unit: "mg" },
    {
      labelKey: "nutrition.potassium",
      value: candidate.potassium_100g,
      unit: "mg",
    },
    {
      labelKey: "nutrition.calcium",
      value: candidate.calcium_100g,
      unit: "mg",
    },
    { labelKey: "nutrition.iron", value: candidate.iron_100g, unit: "mg" },
    {
      labelKey: "nutrition.magnesium",
      value: candidate.magnesium_100g,
      unit: "mg",
    },
    {
      labelKey: "nutrition.phosphorus",
      value: candidate.phosphorus_100g,
      unit: "mg",
    },
    { labelKey: "nutrition.zinc", value: candidate.zinc_100g, unit: "mg" },
  ]

  const vitamins: Row[] = [
    {
      labelKey: "nutrition.vitamin_a",
      value: candidate.vitamin_a_100g,
      unit: "µg",
    },
    {
      labelKey: "nutrition.vitamin_c",
      value: candidate.vitamin_c_100g,
      unit: "mg",
    },
    {
      labelKey: "nutrition.vitamin_d",
      value: candidate.vitamin_d_100g,
      unit: "µg",
    },
    {
      labelKey: "nutrition.vitamin_b12",
      value: candidate.vitamin_b12_100g,
      unit: "µg",
    },
    {
      labelKey: "nutrition.vitamin_b6",
      value: candidate.vitamin_b6_100g,
      unit: "mg",
    },
    { labelKey: "nutrition.folate", value: candidate.folate_100g, unit: "µg" },
  ]

  const hasAnything =
    hasCoreMacros ||
    anyFilled(extended) ||
    anyFilled(minerals) ||
    anyFilled(vitamins)
  if (!hasAnything) return null

  const SourceIcon = candidate.source === "off" ? Globe : Database
  const sourceLabel =
    candidate.source === "off" ? t("lookup.source_off") : t("lookup.source_fdc")

  return (
    <div
      className={cn(
        "space-y-3 rounded-2xl bg-surface-container p-4",
        className
      )}
      data-testid="nutrition-detail"
    >
      <div className="flex items-start gap-3">
        {candidate.image_url && (
          <img
            src={candidate.image_url}
            alt={candidate.name}
            loading="lazy"
            className="size-14 rounded-lg object-cover"
          />
        )}
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex items-center gap-2">
            <p className="truncate text-sm font-semibold text-on-surface">
              {candidate.name}
            </p>
            {recommended && (
              <Sparkles
                className="size-3.5 shrink-0 text-primary"
                aria-label={t("lookup.recommended")}
              />
            )}
          </div>
          {candidate.source_name &&
            candidate.source_name !== candidate.name && (
              <p className="truncate font-mono text-[11px] text-on-surface-variant/80">
                {candidate.source_name}
              </p>
            )}
        </div>
        <Badge
          variant="outline"
          className="shrink-0 gap-1 text-[10px] font-medium tracking-wide"
        >
          <SourceIcon className="size-3" aria-hidden />
          {sourceLabel}
        </Badge>
      </div>

      {candidate.existing_id != null && (
        <div className="flex items-center gap-2 rounded-lg bg-primary-fixed/60 px-3 py-1.5 text-xs text-on-primary-fixed">
          <TriangleAlert className="size-3.5 shrink-0" aria-hidden />
          {t("lookup.existing_warning")}
        </div>
      )}

      {hasCoreMacros && (
        <div className="space-y-2">
          <p className="text-[10px] font-semibold tracking-[0.18em] text-on-surface-variant/70 uppercase">
            {t("nutrition.section_core")}
          </p>
          <div className="flex items-baseline justify-between gap-3">
            <MacroKcalHero
              kcal={candidate.kcal_100g}
              size="sm"
              hint={t("ingredient.per_100g")}
            />
          </div>
          <MacroDistributionBar
            thickness="sm"
            values={{
              protein: candidate.protein_100g,
              carbs: candidate.carbs_100g,
              fat: candidate.fat_100g,
            }}
          />
          <MacroTriad
            size="sm"
            values={{
              protein: candidate.protein_100g,
              carbs: candidate.carbs_100g,
              fat: candidate.fat_100g,
            }}
          />
        </div>
      )}
      <NutrientRow
        rows={extended}
        titleKey="nutrition.section_extended"
        size="sm"
      />
      <NutrientRow
        rows={minerals}
        titleKey="nutrition.section_minerals"
        size="sm"
      />
      <NutrientRow
        rows={vitamins}
        titleKey="nutrition.section_vitamins"
        size="sm"
      />
    </div>
  )
}

function NutrientRow({
  titleKey,
  rows,
  size,
}: {
  titleKey: string
  rows: Row[]
  size: "md" | "sm"
}) {
  const { t } = useTranslation()
  if (!anyFilled(rows)) return null
  const visible = rows.filter((r) => r.value != null)
  return (
    <div className="space-y-1.5">
      <p className="text-[10px] font-semibold tracking-[0.18em] text-on-surface-variant/70 uppercase">
        {t(titleKey)}
      </p>
      <div
        className={cn(
          "flex flex-wrap gap-x-4 gap-y-1 tabular-nums",
          size === "md"
            ? "text-xs text-on-surface-variant"
            : "text-[11px] text-on-surface-variant/80"
        )}
      >
        {visible.map((row) => (
          <span key={row.labelKey} className="inline-flex items-baseline gap-1">
            <span className="text-on-surface-variant/70">
              {t(row.labelKey)}
            </span>
            <span className="font-medium text-on-surface">
              {fmt(row.value as number, row.unit)}
            </span>
          </span>
        ))}
      </div>
    </div>
  )
}
