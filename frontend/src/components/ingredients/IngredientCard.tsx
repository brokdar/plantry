import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { MoreVertical, PencilLine } from "lucide-react"

import { CardImageBadge } from "@/components/editorial/CardImageBadge"
import { EditorialCard } from "@/components/editorial/EditorialCard"
import { FoodPlaceholder } from "@/components/editorial/FoodPlaceholder"
import {
  MacroDistributionBar,
  MacroKcalHero,
  MacroTriad,
} from "@/components/editorial/macros"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { imageURL } from "@/lib/image-url"
import type { LeafFood } from "@/lib/api/foods"

const SOURCE_DOT: Record<string, "primary" | "tertiary" | "muted"> = {
  fdc: "primary",
  off: "tertiary",
  manual: "muted",
}

type IngredientCardProps = {
  ingredient: LeafFood
  onDelete: (id: number) => void
}

export function IngredientCard({ ingredient, onDelete }: IngredientCardProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id, name, source, image_path, updated_at } = ingredient

  const sourceKey = source ?? "manual"
  const sourceDot = SOURCE_DOT[sourceKey] ?? "muted"
  const sourceLabel = t(`ingredient.source_${sourceKey}`, {
    defaultValue: sourceKey,
  })

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
          <EditorialCard.Title className="line-clamp-2 min-h-14">
            {name}
          </EditorialCard.Title>
          <div className="mt-2 flex items-baseline justify-between gap-2">
            <MacroKcalHero
              kcal={ingredient.kcal_100g}
              size="sm"
              hint={t("ingredient.per_100g")}
            />
          </div>
          <MacroDistributionBar
            className="mt-3"
            thickness="sm"
            values={{
              protein: ingredient.protein_100g,
              carbs: ingredient.carbs_100g,
              fat: ingredient.fat_100g,
            }}
          />
          <MacroTriad
            className="mt-3"
            size="xs"
            layout="grid"
            abbreviated
            values={{
              protein: ingredient.protein_100g,
              carbs: ingredient.carbs_100g,
              fat: ingredient.fat_100g,
            }}
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
