import { useState } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import {
  ArrowLeft,
  CalendarPlus,
  CookingPot,
  ExternalLink,
  GitBranch,
  Heart,
  PencilLine,
} from "lucide-react"

import { QuantityStepper } from "@/components/plan/QuantityStepper"
import { useSettingField } from "@/components/settings/setting-field"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { ComposedFood, ScaledIngredient } from "@/lib/api/foods"
import { todayISO } from "@/lib/dates"
import { imageURL } from "@/lib/image-url"
import { MAX_PORTIONS } from "@/lib/portions"
import { formatQuantity, type Formatter } from "@/lib/plan/format"
import {
  useCreateVariant,
  useFoodNutrition,
  useScaled,
  useSetFoodFavorite,
} from "@/lib/queries/foods"
import { lastEatenAgo, rememberedLibrarySearch } from "@/lib/recipes/library"
import { cn } from "@/lib/utils"

import { AddToPlanSheet } from "./AddToPlanSheet"
import { FoodDietSelect } from "./DietClassSelect"
import { DietUnknownWarning } from "./DietUnknownWarning"
import { NutritionCompletenessWarning } from "./NutritionCompletenessWarning"

type RecipeDetailProps = {
  recipe: ComposedFood
  /** `?portions`; the stepper starts here, else at `reference_portions`. */
  portions?: number
  /** The stepper changed; the route keeps it in `?portions`. */
  onPortionsChange?: (portions: number) => void
}

/** Reading view of a recipe (UX §5.8, D6): photo, name, source, facts with
 *  a portion stepper and the diet class (with its override), Add to plan,
 *  the scaled ingredients, the cook history, steps, My
 *  notes and the imported description. Changing portions never writes the
 *  recipe. */
export function RecipeDetail({
  recipe,
  portions: initial,
  onPortionsChange,
}: RecipeDetailProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { data: nutrition } = useFoodNutrition(recipe.id)
  const setFavorite = useSetFoodFavorite()
  const createVariant = useCreateVariant()
  const defaultKeep = useSettingField("plan.default_keep_days", "3")
  const [today] = useState(todayISO)
  const [portions, setPortions] = useState(
    initial ?? recipe.reference_portions ?? 1
  )
  const scaled = useScaled(recipe.id, portions)
  const [backSearch] = useState(rememberedLibrarySearch)
  const [addOpen, setAddOpen] = useState(false)

  function changePortions(next: number) {
    setPortions(next)
    onPortionsChange?.(next)
  }

  const whole = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 0,
  })
  const source = sourceLink(recipe.source_url)
  const steps = (recipe.instructions ?? [])
    .slice()
    .sort((a, b) => a.step_number - b.step_number)
  const notes = recipe.notes?.trim()
  const description = recipe.description?.trim()
  const minutes = (recipe.prep_minutes ?? 0) + (recipe.cook_minutes ?? 0)
  const keepDays = recipe.keep_days ?? Number(defaultKeep.value)
  const ago = lastEatenAgo(recipe.last_eaten, today, i18n.language)
  const history =
    recipe.times_cooked === 0
      ? t("recipe_detail.history_never")
      : ago
        ? t("recipe_detail.history", { count: recipe.times_cooked, when: ago })
        : t("recipe_detail.history_count", { count: recipe.times_cooked })

  return (
    <article className="mx-auto flex max-w-5xl flex-col gap-8 px-4 py-6 text-ink md:px-8 md:py-10">
      <div className="flex flex-col items-start gap-3 md:flex-row md:items-center md:justify-between">
        <Link
          to="/recipes"
          search={backSearch}
          className="inline-flex items-center gap-1.5 rounded-md text-sm whitespace-nowrap text-ink-muted hover:text-ink focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none max-md:min-h-11"
        >
          <ArrowLeft className="size-4" strokeWidth={1.5} aria-hidden />
          {t("recipe_detail.back")}
        </Link>
        {/* Below md one row: Cook now first and wide, the rest icon-only. */}
        <div
          className="flex w-full items-center gap-2 md:w-auto md:flex-wrap md:justify-end"
          data-testid="recipe-actions"
        >
          <Button
            type="button"
            variant="outline"
            aria-pressed={recipe.favorite}
            disabled={setFavorite.isPending}
            onClick={() =>
              setFavorite.mutate({ id: recipe.id, favorite: !recipe.favorite })
            }
            className="max-md:size-11"
            data-testid="recipe-favorite"
          >
            <Heart
              data-icon="inline-start"
              strokeWidth={1.5}
              className={cn(recipe.favorite && "fill-current text-accent")}
              aria-hidden
            />
            <span className="max-md:sr-only">
              {t("recipe_detail.favorite")}
            </span>
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={createVariant.isPending}
            onClick={() =>
              createVariant.mutate(recipe.id, {
                onSuccess: (variant) =>
                  void navigate({
                    to: "/recipes/$id/edit",
                    params: { id: String(variant.id) },
                  }),
              })
            }
            className="max-md:size-11"
            data-testid="recipe-create-variant"
          >
            <GitBranch data-icon="inline-start" strokeWidth={1.5} aria-hidden />
            <span className="max-md:sr-only">
              {t("component.create_variant")}
            </span>
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => setAddOpen(true)}
            className="max-md:size-11"
            data-testid="recipe-add-to-plan"
          >
            <CalendarPlus
              data-icon="inline-start"
              strokeWidth={1.5}
              aria-hidden
            />
            <span className="max-md:sr-only">{t("add_to_plan.action")}</span>
          </Button>
          <Button asChild variant="outline" className="max-md:size-11">
            <Link
              to="/recipes/$id/edit"
              params={{ id: String(recipe.id) }}
              data-testid="recipe-edit"
            >
              <PencilLine
                data-icon="inline-start"
                strokeWidth={1.5}
                aria-hidden
              />
              <span className="max-md:sr-only">{t("common.edit")}</span>
            </Link>
          </Button>
          <Button
            asChild
            className="max-md:order-first max-md:min-h-11 max-md:flex-1"
          >
            <Link
              to="/recipes/$id/cook"
              params={{ id: String(recipe.id) }}
              search={{ portions }}
              data-testid="recipe-cook-now"
            >
              <CookingPot
                data-icon="inline-start"
                strokeWidth={1.5}
                aria-hidden
              />
              {t("recipe_detail.cook_now")}
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
              className="inline-flex w-fit items-center gap-1.5 rounded-md text-sm text-accent underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none max-md:min-h-11"
              data-testid="recipe-source"
            >
              <span>{t("recipe_detail.source", { host: source.host })}</span>
              <span className="sr-only">{t("common.opens_new_tab")}</span>
              <ExternalLink
                className="size-3.5"
                strokeWidth={1.5}
                aria-hidden
              />
            </a>
          )}
          <dl
            className="flex flex-wrap items-start gap-x-8 gap-y-4 border-t border-line pt-4"
            data-testid="recipe-facts"
          >
            {minutes > 0 && (
              <Fact label={t("recipe_detail.time")}>
                {t("recipe_detail.minutes", { count: minutes })}
              </Fact>
            )}
            <div className="flex flex-col gap-0.5">
              <dt className="text-small text-ink-muted">
                {t("recipe_detail.portions")}
              </dt>
              <dd>
                <QuantityStepper
                  label={t("recipe_detail.portions")}
                  value={portions}
                  min={1}
                  max={MAX_PORTIONS}
                  step={1}
                  onChange={changePortions}
                  testId="recipe-portions"
                />
              </dd>
            </div>
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
            <div className="flex flex-col gap-0.5">
              <dt className="text-small text-ink-muted">{t("diet.label")}</dt>
              <dd>
                <FoodDietSelect
                  food={recipe}
                  label={t("diet.for", { name: recipe.name })}
                  className="md:pointer-fine:min-h-9"
                  testId="recipe-diet"
                />
              </dd>
            </div>
            <Fact label={t("recipe_detail.keeps")}>
              {t("recipe_detail.days", { count: keepDays })}
            </Fact>
            {recipe.freezable != null && (
              <Fact label={t("recipe_detail.freezable")}>
                {recipe.freezable ? t("common.yes") : t("common.no")}
              </Fact>
            )}
          </dl>
          <p className="text-small text-ink-muted" data-testid="recipe-history">
            {history}
          </p>
          {nutrition && !nutrition.complete && (
            <NutritionCompletenessWarning missing={nutrition.missing} />
          )}
          {recipe.diet_class === "unknown" && scaled.data && (
            <DietUnknownWarning
              unknown={scaled.data.ingredients.filter(
                (i) => i.diet_class === "unknown"
              )}
            />
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
          {(recipe.children ?? []).length === 0 ? (
            <p className="text-sm text-ink-muted">
              {t("recipe_detail.no_ingredients")}
            </p>
          ) : !scaled.data && scaled.isError ? (
            <div
              role="alert"
              className="flex flex-col items-start gap-2 rounded-lg border border-line bg-panel p-4"
              data-testid="recipe-ingredients-error"
            >
              <p className="text-body text-ink">
                {t("recipe_detail.ingredients_error")}
              </p>
              <Button
                type="button"
                variant="outline"
                className="max-md:min-h-11"
                onClick={() => void scaled.refetch()}
              >
                {t("common.retry")}
              </Button>
            </div>
          ) : !scaled.data ? (
            <div
              className="flex flex-col gap-2"
              data-testid="recipe-ingredients-loading"
            >
              {(recipe.children ?? []).map((c) => (
                <Skeleton key={c.id} className="h-6 w-full" />
              ))}
            </div>
          ) : (
            <ul
              className={cn(
                "flex flex-col divide-y divide-line border-y border-line transition-opacity",
                scaled.isPlaceholderData && "opacity-60"
              )}
              aria-busy={scaled.isFetching}
              data-testid="recipe-ingredients-list"
            >
              {scaled.data.ingredients.map((ing) => (
                <IngredientLine key={ing.child_id} ing={ing} f={i18n} />
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
              {t("recipe_detail.steps")}
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
      <AddToPlanSheet
        recipe={recipe}
        open={addOpen}
        onOpenChange={setAddOpen}
      />
    </article>
  )
}

function IngredientLine({ ing, f }: { ing: ScaledIngredient; f: Formatter }) {
  const { t } = useTranslation()
  const shown = formatQuantity(ing.display.amount, ing.display.unit, f)
  const buy =
    ing.buy &&
    (ing.buy.amount !== ing.display.amount || ing.buy.unit !== ing.display.unit)
      ? formatQuantity(ing.buy.amount, ing.buy.unit, f)
      : null
  return (
    <li className="flex gap-3 py-2.5">
      <span className="w-20 shrink-0 text-right text-ink-muted tabular-nums">
        {shown}
      </span>
      <span className="flex flex-col">
        <span>{ing.name}</span>
        {buy && (
          <span className="text-small text-ink-muted">
            {t("recipe_detail.buy", { quantity: buy })}
          </span>
        )}
      </span>
    </li>
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
      {/* As tall as the portion stepper, so values line up with it. */}
      <dd className="flex min-h-11 items-center text-h3 font-semibold tabular-nums md:pointer-fine:min-h-9">
        {children}
      </dd>
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
