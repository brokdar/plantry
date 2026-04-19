import { useState } from "react"
import { useTranslation } from "react-i18next"

import { MacroBar } from "@/components/editorial/MacroBar"
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

  const proteinKcal = macros.protein * 4
  const carbsKcal = macros.carbs * 4
  const fatKcal = macros.fat * 9
  const totalKcal = Math.max(proteinKcal + carbsKcal + fatKcal, 1)

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
      <div className="space-y-1">
        <p className="font-heading text-5xl font-extrabold text-on-surface">
          {Math.round(macros.kcal)}
          <span className="ml-2 text-sm font-medium tracking-widest text-on-surface-variant uppercase">
            kcal
          </span>
        </p>
        <p className="text-xs tracking-widest text-on-surface-variant uppercase">
          {view === "portion"
            ? t("component.nutrition_per_portion_hint")
            : t("component.nutrition_total_hint", { count: portions })}
        </p>
      </div>

      <MacroBar
        thickness="lg"
        track="surface-container-highest"
        segments={[
          {
            value: proteinKcal,
            color: "primary",
            label: t("ingredient.protein"),
          },
          { value: carbsKcal, color: "tertiary", label: t("ingredient.carbs") },
          { value: fatKcal, color: "secondary", label: t("ingredient.fat") },
        ]}
        max={totalKcal}
      />

      <div className="grid grid-cols-3 gap-2">
        <MacroStat
          label={t("ingredient.protein")}
          value={macros.protein}
          dot="primary"
        />
        <MacroStat
          label={t("ingredient.carbs")}
          value={macros.carbs}
          dot="tertiary"
        />
        <MacroStat
          label={t("ingredient.fat")}
          value={macros.fat}
          dot="secondary"
        />
      </div>

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

function MacroStat({
  label,
  value,
  dot,
}: {
  label: string
  value: number
  dot: "primary" | "tertiary" | "secondary"
}) {
  const dotClass =
    dot === "primary"
      ? "bg-primary"
      : dot === "tertiary"
        ? "bg-tertiary"
        : "bg-outline"
  return (
    <div className="rounded-xl bg-surface-container px-3 py-2">
      <p className="flex items-center gap-1.5 text-[10px] tracking-widest text-on-surface-variant uppercase">
        <span className={cn("inline-block size-1.5 rounded-full", dotClass)} />
        {label}
      </p>
      <p className="mt-0.5 font-heading text-lg font-bold text-on-surface">
        {value.toFixed(1)}
        <span className="ml-1 text-xs font-medium text-on-surface-variant">
          g
        </span>
      </p>
    </div>
  )
}
