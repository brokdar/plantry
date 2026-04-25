import { Heart } from "lucide-react"
import { useTranslation } from "react-i18next"

import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import type { Food } from "@/lib/api/foods"
import { imageURL } from "@/lib/image-url"
import { cn } from "@/lib/utils"

interface PickerCardProps {
  component: Food
  onPick: () => void
  onToggleFavorite: () => void
}

export function PickerCard({
  component,
  onPick,
  onToggleFavorite,
}: PickerCardProps) {
  const { t } = useTranslation()

  const chipLabel =
    component.kind === "leaf"
      ? t("ingredient.kind_label", { defaultValue: "Lebensmittel" })
      : t(`planner.slot.role.${component.role}`, {
          defaultValue: component.role,
        })

  const prepMins =
    component.kind === "composed" ? (component.prep_minutes ?? 0) : 0
  const cookMins =
    component.kind === "composed" ? (component.cook_minutes ?? 0) : 0
  const totalMins = prepMins + cookMins

  return (
    <div
      data-testid={`picker-card-${component.id}`}
      className="group relative flex cursor-pointer flex-col overflow-hidden rounded-[14px] border border-outline-variant/50 bg-surface-container-lowest transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-[0_1px_2px_rgba(25,28,28,0.04),0_4px_12px_-4px_rgba(25,28,28,0.06)]"
      onClick={onPick}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault()
          onPick()
        }
      }}
      role="button"
      tabIndex={0}
      aria-label={component.name}
    >
      <div className="relative aspect-[4/3] overflow-hidden">
        {component.image_path ? (
          <img
            src={imageURL(component.image_path)}
            alt=""
            className="h-full w-full object-cover"
          />
        ) : (
          <FoodPlaceholder
            category={
              ((component.kind === "composed" ? component.role : null) ??
                "main") as FoodPlaceholderCategory
            }
            size="lg"
            rounded="none"
            className="h-full w-full"
          />
        )}
        <span className="absolute bottom-2 left-2 rounded-full bg-white/90 px-2 py-0.5 font-heading text-[9.5px] font-bold tracking-[0.14em] text-on-surface uppercase">
          {chipLabel}
        </span>
        <button
          type="button"
          aria-label={t("planner.slot.action.favorite")}
          aria-pressed={component.favorite}
          onClick={(e) => {
            e.stopPropagation()
            onToggleFavorite()
          }}
          className={cn(
            "absolute top-2 right-2 grid size-7 place-items-center rounded-full bg-white/90 text-on-surface-variant",
            component.favorite && "text-[#c44a4a]"
          )}
        >
          <Heart
            className="h-3.5 w-3.5"
            fill={component.favorite ? "currentColor" : "none"}
            aria-hidden
          />
        </button>
      </div>
      <div className="flex flex-col gap-1 p-3.5">
        <p className="font-heading text-[14px] leading-tight font-bold tracking-tight">
          {component.name}
        </p>
        {component.kind === "leaf" ? (
          <p className="text-[11.5px] text-on-surface-variant">
            {component.kcal_100g != null && (
              <span>{component.kcal_100g} kcal</span>
            )}
            {component.kcal_100g != null && component.protein_100g != null && (
              <span className="mx-1 inline-block size-0.5 rounded-full bg-outline-variant align-middle" />
            )}
            {component.protein_100g != null && (
              <span>
                {component.protein_100g}g{" "}
                {t("nutrition.protein", { defaultValue: "protein" })}
              </span>
            )}
          </p>
        ) : totalMins > 0 || component.cook_count > 0 ? (
          <p className="flex items-center gap-1.5 text-[11.5px] text-on-surface-variant">
            {totalMins > 0 && <span>{totalMins} min</span>}
            {totalMins > 0 && component.cook_count > 0 && (
              <span className="size-0.5 rounded-full bg-outline-variant" />
            )}
            {component.cook_count > 0 && (
              <span>
                {t("picker.cooked_count", {
                  count: component.cook_count,
                  defaultValue: "cooked {{count}}×",
                })}
              </span>
            )}
          </p>
        ) : (
          <p className="text-[11.5px] text-on-surface-variant/70 italic">
            {t("picker.card.new", { defaultValue: "New — not yet cooked" })}
          </p>
        )}
        {component.kind === "composed" && (component.tags?.length ?? 0) > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {(component.tags ?? []).slice(0, 3).map((tag) => (
              <span
                key={tag}
                className="rounded-full bg-surface-container-low px-2 py-0.5 font-heading text-[10px] font-semibold tracking-wide text-on-surface-variant"
              >
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
