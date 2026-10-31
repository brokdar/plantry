import { Link } from "@tanstack/react-router"
import { Heart } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { ComposedFood } from "@/lib/api/foods"
import { imageURL } from "@/lib/image-url"
import { lastEatenAgo } from "@/lib/recipes/library"

import { DietIcon } from "./DietIcon"

type RecipeCardProps = {
  recipe: ComposedFood
  /** YYYY-MM-DD, the reference day for "last eaten". */
  today: string
}

/**
 * A library card (UX §5.8): 4:3 photo or the typographic placeholder (the
 * initial on the Herb tint, as on recipe detail), the name in Newsreader and
 * one meta line led by the diet icon: time, kcal per portion and when it was
 * last eaten.
 */
export function RecipeCard({ recipe, today }: RecipeCardProps) {
  const { t, i18n } = useTranslation()
  const { id, name, image_path, updated_at, per_portion } = recipe
  const minutes = (recipe.prep_minutes ?? 0) + (recipe.cook_minutes ?? 0)
  const ago = lastEatenAgo(recipe.last_eaten, today, i18n.language)
  const kcal = Math.round(per_portion.kcal)

  const meta = [
    minutes > 0 ? t("recipes.card.minutes", { count: minutes }) : null,
    kcal > 0
      ? t("recipes.card.kcal", {
          kcal: new Intl.NumberFormat(i18n.language).format(kcal),
        })
      : null,
    ago ? t("recipes.card.eaten", { when: ago }) : t("recipes.card.never"),
  ].filter(Boolean)

  return (
    <Link
      to="/recipes/$id"
      params={{ id: String(id) }}
      className="group flex flex-col gap-2 rounded-lg text-ink"
      data-testid={`recipe-card-${id}`}
    >
      <div className="relative">
        {image_path ? (
          <img
            src={imageURL(image_path, updated_at)}
            alt=""
            loading="lazy"
            className="aspect-[4/3] w-full rounded-lg border border-line object-cover"
          />
        ) : (
          <div
            aria-hidden
            className="flex aspect-[4/3] w-full items-center justify-center rounded-lg bg-batch-3-tint font-heading text-5xl text-batch-3"
          >
            {name.trim().charAt(0).toUpperCase()}
          </div>
        )}
        {recipe.favorite && (
          <span className="absolute top-2 right-2 flex size-7 items-center justify-center rounded-full bg-panel text-ink">
            <Heart
              className="size-4 fill-current"
              strokeWidth={1.5}
              aria-label={t("recipes.card.favorite")}
            />
          </span>
        )}
      </div>
      <h2 className="line-clamp-2 font-heading text-h3 leading-snug font-medium group-hover:underline group-hover:underline-offset-2">
        {name}
      </h2>
      <p
        className="flex items-start gap-1.5 text-small text-ink-muted tabular-nums"
        data-testid={`recipe-card-${id}-meta`}
      >
        <DietIcon dietClass={recipe.diet_class} className="mt-0.5" />
        <span>{meta.join(" · ")}</span>
      </p>
    </Link>
  )
}
