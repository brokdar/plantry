import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { MoreVertical, PencilLine } from "lucide-react"

import { CardImageBadge } from "@/components/editorial/CardImageBadge"
import { EditorialCard } from "@/components/editorial/EditorialCard"
import { FoodPlaceholder } from "@/components/editorial/FoodPlaceholder"
import { MacroBar } from "@/components/editorial/MacroBar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { imageURL } from "@/lib/image-url"
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
      <EditorialCard interactive>
        <Link
          to="/ingredients/$id/edit"
          params={{ id: String(id) }}
          className="absolute inset-0 z-0"
          aria-label={name}
        />
        <div className="pointer-events-none relative">
          {image_path ? (
            <EditorialCard.Image
              src={imageURL(image_path, updated_at)}
              alt={name}
            />
          ) : (
            <FoodPlaceholder
              category="ingredient"
              className="m-2 aspect-[4/3] w-[calc(100%-1rem)]"
              aria-label={name}
            />
          )}
          <EditorialCard.ImageOverlay position="bottom-left">
            <CardImageBadge dot={sourceDot}>{sourceLabel}</CardImageBadge>
          </EditorialCard.ImageOverlay>
          <EditorialCard.ImageOverlay
            position="top-right"
            className="opacity-100 transition-opacity duration-200 md:opacity-0 md:group-focus-within:opacity-100 md:group-hover:opacity-100"
          >
            <span
              aria-hidden
              className="hidden size-7 items-center justify-center rounded-full bg-surface-container-lowest/85 backdrop-blur-md md:flex"
            >
              <PencilLine className="size-3.5 text-on-surface" />
            </span>
          </EditorialCard.ImageOverlay>
        </div>
        <EditorialCard.Body>
          <EditorialCard.Title className="line-clamp-2">
            {name}
          </EditorialCard.Title>
          <EditorialCard.Meta className="mt-2 flex-wrap gap-x-3 gap-y-1.5">
            <Badge variant="secondary">
              {fmt(ingredient.kcal_100g, " kcal")}
            </Badge>
            <span>
              {t("ingredient.protein").charAt(0)}{" "}
              {fmt(ingredient.protein_100g, "g")}
            </span>
            <span>
              {t("ingredient.fat").charAt(0)} {fmt(ingredient.fat_100g, "g")}
            </span>
            <span>
              {t("ingredient.carbs").charAt(0)}{" "}
              {fmt(ingredient.carbs_100g, "g")}
            </span>
          </EditorialCard.Meta>
          <MacroBar
            className="mt-3"
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
        </EditorialCard.Body>
      </EditorialCard>
      <div className="absolute top-3 right-3 z-30 opacity-100 transition-opacity duration-200 focus-within:opacity-100 md:opacity-0 md:group-focus-within:opacity-100 md:group-hover:opacity-100">
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
