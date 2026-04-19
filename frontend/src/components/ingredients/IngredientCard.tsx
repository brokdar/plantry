import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { MoreVertical, Package } from "lucide-react"

import { CardImageBadge } from "@/components/editorial/CardImageBadge"
import { MacroBar } from "@/components/editorial/MacroBar"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { imageURL } from "@/lib/image-url"
import { cn } from "@/lib/utils"
import type { Ingredient } from "@/lib/api/ingredients"

const SOURCE_DOT: Record<string, "primary" | "tertiary" | "muted"> = {
  fdc: "primary",
  off: "tertiary",
  manual: "muted",
}

type IngredientCardProps = {
  ingredient: Ingredient
  onDelete: (id: number) => void
}

function fmt(value: number | null | undefined, suffix = ""): string {
  if (value == null) return "—"
  return `${Math.round(value * 10) / 10}${suffix}`
}

export function IngredientCard({ ingredient, onDelete }: IngredientCardProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id, name, source, image_path, updated_at } = ingredient

  const sourceDot = SOURCE_DOT[source] ?? "muted"
  const sourceLabel = t(`ingredient.source_${source}`, { defaultValue: source })

  const proteinKcal = (ingredient.protein_100g ?? 0) * 4
  const carbsKcal = (ingredient.carbs_100g ?? 0) * 4
  const fatKcal = (ingredient.fat_100g ?? 0) * 9
  const totalKcal = Math.max(proteinKcal + carbsKcal + fatKcal, 1)

  return (
    <div className="group relative" data-testid={`ingredient-card-${id}`}>
      <article
        className={cn(
          "editorial-shadow relative flex flex-col overflow-hidden rounded-2xl bg-surface-container-lowest",
          "transition-all duration-200 hover:-translate-y-0.5 hover:shadow-lg",
          "focus-within:ring-2 focus-within:ring-primary"
        )}
      >
        <Link
          to="/ingredients/$id/edit"
          params={{ id: String(id) }}
          className="absolute inset-0 z-10"
          aria-label={name}
        />
        <div className="pointer-events-none relative aspect-[3/4] overflow-hidden bg-surface-container-high">
          {image_path ? (
            <img
              src={imageURL(image_path, updated_at)}
              alt=""
              className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
              loading="lazy"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center">
              <Package
                className="size-10 text-on-surface-variant/30"
                aria-hidden
              />
            </div>
          )}
          <div className="absolute bottom-2 left-2 flex gap-1.5">
            <CardImageBadge dot={sourceDot}>{sourceLabel}</CardImageBadge>
          </div>
        </div>
        <div className="pointer-events-none flex flex-1 flex-col gap-2 p-3">
          <p className="line-clamp-2 font-heading text-sm leading-tight font-semibold text-on-surface">
            {name}
          </p>
          <div className="flex flex-wrap gap-1">
            <MacroPill>{fmt(ingredient.kcal_100g, " kcal")}</MacroPill>
            <MacroPill>
              {t("ingredient.protein").charAt(0)}{" "}
              {fmt(ingredient.protein_100g, "g")}
            </MacroPill>
            <MacroPill>
              {t("ingredient.fat").charAt(0)} {fmt(ingredient.fat_100g, "g")}
            </MacroPill>
            <MacroPill>
              {t("ingredient.carbs").charAt(0)}{" "}
              {fmt(ingredient.carbs_100g, "g")}
            </MacroPill>
          </div>
          <MacroBar
            thickness="sm"
            track="surface-container-highest"
            segments={[
              {
                value: proteinKcal,
                color: "primary",
                label: t("ingredient.protein"),
              },
              {
                value: carbsKcal,
                color: "tertiary",
                label: t("ingredient.carbs"),
              },
              {
                value: fatKcal,
                color: "secondary",
                label: t("ingredient.fat"),
              },
            ]}
            max={totalKcal}
          />
        </div>
      </article>
      <div className="absolute top-2 right-2 z-20 opacity-100 transition-opacity duration-200 focus-within:opacity-100 md:opacity-0 md:group-focus-within:opacity-100 md:group-hover:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("common.actions")}
              className="bg-surface-container-lowest/85 backdrop-blur-md"
              data-testid={`ingredient-card-${id}-menu`}
            >
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onClick={() =>
                navigate({
                  to: "/ingredients/$id/edit",
                  params: { id: String(id) },
                })
              }
            >
              {t("common.edit")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onDelete(id)}
              className="text-destructive focus:text-destructive"
              data-testid={`ingredient-card-${id}-delete`}
            >
              {t("common.delete")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}

function MacroPill({ children }: { children: React.ReactNode }) {
  return (
    <span className="rounded-full bg-surface-container px-2 py-0.5 text-[10px] font-medium text-on-surface-variant">
      {children}
    </span>
  )
}
