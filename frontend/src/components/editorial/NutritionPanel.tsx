import { useState } from "react"
import { useTranslation } from "react-i18next"

import {
  MacroDistributionBar,
  MacroKcalHero,
  MacroTriad,
} from "@/components/editorial/macros"
import { SectionCard } from "@/components/editorial/SectionCard"
import { cn } from "@/lib/utils"

type Macros = {
  kcal: number
  protein: number
  fat: number
  carbs: number
  fiber: number
  sodium: number
}

type NutritionPanelProps = {
  perPortion: Macros
  referencePortions: number
  className?: string
}

export function NutritionPanel({
  perPortion,
  referencePortions,
  className,
}: NutritionPanelProps) {
  const { t } = useTranslation()
  const [view, setView] = useState<"portion" | "total">("portion")
  const portions = Math.max(referencePortions, 1)
  const macros: Macros =
    view === "portion"
      ? perPortion
      : {
          kcal: perPortion.kcal * portions,
          protein: perPortion.protein * portions,
          fat: perPortion.fat * portions,
          carbs: perPortion.carbs * portions,
          fiber: perPortion.fiber * portions,
          sodium: perPortion.sodium * portions,
        }

  return (
    <SectionCard
      className={cn("h-fit", className)}
      title={t("component.nutrition")}
      testId="section-card-nutrition"
      actions={
        <div
          role="group"
          aria-label={t("component.nutrition_view_label")}
          className="inline-flex items-center rounded-full bg-surface-container-highest p-0.5 text-xs"
        >
          <ToggleButton
            active={view === "portion"}
            onClick={() => setView("portion")}
            testId="nutrition-view-portion"
          >
            {t("component.nutrition_per_portion")}
          </ToggleButton>
          <ToggleButton
            active={view === "total"}
            onClick={() => setView("total")}
            testId="nutrition-view-total"
          >
            {t("component.nutrition_total")}
          </ToggleButton>
        </div>
      }
    >
      <MacroKcalHero
        kcal={macros.kcal}
        size="lg"
        hint={
          view === "portion"
            ? t("component.nutrition_per_portion_hint")
            : t("component.nutrition_total_hint", { count: portions })
        }
      />

      <MacroDistributionBar
        thickness="lg"
        values={{
          protein: macros.protein,
          carbs: macros.carbs,
          fat: macros.fat,
        }}
      />

      <MacroTriad
        size="md"
        values={{
          protein: macros.protein,
          carbs: macros.carbs,
          fat: macros.fat,
        }}
      />

      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-on-surface-variant">
        <div className="flex items-center justify-between">
          <dt>{t("ingredient.fiber")}</dt>
          <dd className="font-medium text-on-surface">
            {macros.fiber.toFixed(1)} g
          </dd>
        </div>
        <div className="flex items-center justify-between">
          <dt>{t("ingredient.sodium")}</dt>
          <dd className="font-medium text-on-surface">
            {(macros.sodium * 1000).toFixed(0)} mg
          </dd>
        </div>
      </dl>
    </SectionCard>
  )
}

function ToggleButton({
  active,
  onClick,
  children,
  testId,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
  testId?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      data-testid={testId}
      className={cn(
        "rounded-full px-3 py-1 transition-colors",
        active
          ? "bg-surface-container-lowest text-on-surface shadow-sm"
          : "text-on-surface-variant hover:text-on-surface"
      )}
    >
      {children}
    </button>
  )
}
