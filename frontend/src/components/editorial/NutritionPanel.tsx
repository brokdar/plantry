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

type ToggleableProps = {
  variant?: "toggleable"
  macros: Macros
  referencePortions: number
}

type StaticProps = {
  variant: "static"
  macros: Macros
  hint?: string
  referencePortions?: never
}

type NutritionPanelProps = (ToggleableProps | StaticProps) & {
  title?: string
  showFiberSodium?: boolean
  testId?: string
  className?: string
}

export function NutritionPanel(props: NutritionPanelProps) {
  const { t } = useTranslation()
  const {
    title = t("component.nutrition"),
    showFiberSodium = true,
    className,
    testId = "section-card-nutrition",
  } = props

  const isToggleable = props.variant !== "static"
  const [view, setView] = useState<"portion" | "total">("portion")
  const portions = isToggleable ? Math.max(props.referencePortions, 1) : 1

  const macros: Macros =
    !isToggleable || view === "portion"
      ? props.macros
      : {
          kcal: props.macros.kcal * portions,
          protein: props.macros.protein * portions,
          fat: props.macros.fat * portions,
          carbs: props.macros.carbs * portions,
          fiber: props.macros.fiber * portions,
          sodium: props.macros.sodium * portions,
        }

  const heroHint = isToggleable
    ? view === "portion"
      ? t("component.nutrition_per_portion_hint")
      : t("component.nutrition_total_hint", { count: portions })
    : props.variant === "static"
      ? props.hint
      : undefined

  return (
    <SectionCard
      className={cn("h-fit", className)}
      title={title}
      testId={testId}
      actions={
        isToggleable ? (
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
        ) : undefined
      }
    >
      <MacroKcalHero kcal={macros.kcal} size="lg" hint={heroHint} />

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

      {showFiberSodium && (
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
      )}
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
