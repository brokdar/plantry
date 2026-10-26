import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ArrowLeft, ExternalLink, Heart, PencilLine } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { ComposedFood } from "@/lib/api/foods"
import { imageURL } from "@/lib/image-url"
import { useFoodNutrition, useSetFoodFavorite } from "@/lib/queries/foods"
import { cn } from "@/lib/utils"

import { NutritionCompletenessWarning } from "./NutritionCompletenessWarning"

type RecipeDetailProps = {
  recipe: ComposedFood
}

/** Reading view of a recipe (D7, Phase 0 subset): photo, name, source,
 *  per-portion facts, ingredients, steps, My notes and the imported
 *  description. */
export function RecipeDetail({ recipe }: RecipeDetailProps) {
  const { t, i18n } = useTranslation()
  const { data: nutrition } = useFoodNutrition(recipe.id)
  const setFavorite = useSetFoodFavorite()

  const number = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 1,
  })
  const whole = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 0,
  })
  const source = sourceLink(recipe.source_url)
  const children = recipe.children ?? []
  const steps = (recipe.instructions ?? [])
    .slice()
    .sort((a, b) => a.step_number - b.step_number)
  const notes = recipe.notes?.trim()
  const description = recipe.description?.trim()

  return (
    <article className="mx-auto flex max-w-5xl flex-col gap-8 px-4 py-6 text-ink md:px-8 md:py-10">
      <div className="flex items-center justify-between gap-3">
        <Link
          to="/components"
          className="inline-flex items-center gap-1.5 rounded-md text-sm text-ink-muted hover:text-ink focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
        >
          <ArrowLeft className="size-4" strokeWidth={1.5} aria-hidden />
          {t("component.breadcrumb_back")}
        </Link>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            aria-pressed={recipe.favorite}
            disabled={setFavorite.isPending}
            onClick={() =>
              setFavorite.mutate({ id: recipe.id, favorite: !recipe.favorite })
            }
            data-testid="recipe-favorite"
          >
            <Heart
              data-icon="inline-start"
              strokeWidth={1.5}
              className={cn(recipe.favorite && "fill-current text-accent")}
              aria-hidden
            />
            {t("recipe_detail.favorite")}
          </Button>
          <Button asChild>
            <Link
              to="/components/$id/edit"
              params={{ id: String(recipe.id) }}
              data-testid="recipe-edit"
            >
              <PencilLine
                data-icon="inline-start"
                strokeWidth={1.5}
                aria-hidden
              />
              {t("common.edit")}
            </Link>
          </Button>
        </div>
      </div>

      <header className="grid gap-6 md:grid-cols-12 md:items-start md:gap-8">
        <div className="md:col-span-5">
          {recipe.image_path ? (
            <img
              src={imageURL(recipe.image_path, recipe.updated_at)}
              alt=""
              className="aspect-[4/3] w-full rounded-lg border border-line object-cover"
            />
          ) : (
            <div
              aria-hidden
              className="flex aspect-[4/3] w-full items-center justify-center rounded-lg bg-batch-3-tint font-heading text-7xl text-batch-3"
            >
              {recipe.name.trim().charAt(0).toUpperCase()}
            </div>
          )}
        </div>
        <div className="flex flex-col gap-4 md:col-span-7">
          <h1 className="font-heading text-h1 leading-tight font-semibold text-balance">
            {recipe.name}
          </h1>
          {source && (
            <a
              href={source.href}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex w-fit items-center gap-1.5 text-sm text-accent underline-offset-2 hover:underline"
              data-testid="recipe-source"
            >
              <span>{t("recipe_detail.source", { host: source.host })}</span>
              <ExternalLink
                className="size-3.5"
                strokeWidth={1.5}
                aria-hidden
              />
            </a>
          )}
          <dl
            className="flex flex-wrap gap-x-8 gap-y-3 border-t border-line pt-4"
            data-testid="recipe-facts"
          >
            <Fact label={t("recipe_detail.portions")}>
              {number.format(recipe.reference_portions ?? 1)}
            </Fact>
            <Fact label={t("recipe_detail.kcal_per_portion")}>
              {nutrition ? whole.format(nutrition.kcal) : "–"}
            </Fact>
            <Fact label={t("recipe_detail.protein_per_portion")}>
              {nutrition
                ? t("recipe_detail.grams", {
                    value: whole.format(nutrition.protein),
                  })
                : "–"}
            </Fact>
          </dl>
          {nutrition && !nutrition.complete && (
            <NutritionCompletenessWarning missing={nutrition.missing} />
          )}
        </div>
      </header>

      <div className="grid gap-10 md:grid-cols-12 md:gap-8">
        <section
          aria-labelledby="recipe-ingredients"
          className="flex flex-col gap-3 md:col-span-4"
        >
          <h2
            id="recipe-ingredients"
            className="font-heading text-h2 font-semibold"
          >
            {t("component.ingredients")}
          </h2>
          {children.length === 0 ? (
            <p className="text-sm text-ink-muted">
              {t("recipe_detail.no_ingredients")}
            </p>
          ) : (
            <ul className="flex flex-col divide-y divide-line border-y border-line">
              {children.map((child) => (
                <li key={child.id} className="flex gap-3 py-2.5">
                  <span className="w-20 shrink-0 text-right text-ink-muted tabular-nums">
                    {number.format(child.amount)} {child.unit}
                  </span>
                  <span>{child.child_name}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <div className="flex flex-col gap-10 md:col-span-8">
          <section
            aria-labelledby="recipe-steps"
            className="flex flex-col gap-3"
          >
            <h2
              id="recipe-steps"
              className="font-heading text-h2 font-semibold"
            >
              {t("component.instructions")}
            </h2>
            {steps.length === 0 ? (
              <p className="text-sm text-ink-muted">
                {t("recipe_detail.no_steps")}
              </p>
            ) : (
              <ol className="flex max-w-[68ch] flex-col gap-4">
                {steps.map((step, i) => (
                  <li key={step.id} className="flex gap-4">
                    <span className="w-6 shrink-0 pt-px text-right font-semibold text-ink-muted tabular-nums">
                      {i + 1}
                    </span>
                    <p className="leading-relaxed whitespace-pre-line">
                      {step.text}
                    </p>
                  </li>
                ))}
              </ol>
            )}
          </section>

          {notes && (
            <section
              aria-labelledby="recipe-notes-title"
              className="flex max-w-[68ch] flex-col gap-2 rounded-lg bg-panel-sunken px-5 py-4"
              data-testid="recipe-notes"
            >
              <h2
                id="recipe-notes-title"
                className="font-heading text-h3 font-semibold"
              >
                {t("recipe_detail.my_notes")}
              </h2>
              <p className="leading-relaxed whitespace-pre-line">{notes}</p>
            </section>
          )}

          {description && (
            <section
              aria-label={t("recipe_detail.description")}
              className="max-w-[68ch] text-sm leading-relaxed whitespace-pre-line text-ink-muted"
              data-testid="recipe-description"
            >
              {description}
            </section>
          )}
        </div>
      </div>
    </article>
  )
}

function Fact({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-small text-ink-muted">{label}</dt>
      <dd className="text-h3 font-semibold tabular-nums">{children}</dd>
    </div>
  )
}

/** Only http(s) sources become links; the host is the visible label. */
function sourceLink(
  url: string | null | undefined
): { href: string; host: string } | null {
  if (!url) return null
  try {
    const parsed = new URL(url)
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return null
    return { href: url, host: parsed.hostname.replace(/^www\./, "") }
  } catch {
    return null
  }
}
