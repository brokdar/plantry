import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { ReviewFinding, ReviewRecipe } from "@/lib/api/library-review"
import { useSetFoodArchived } from "@/lib/queries/foods"
import {
  useLibraryReview,
  useLibraryReviewActions,
} from "@/lib/queries/library-review"

/** The monthly library review (LIB-1..3, Chef on and the review switched
 *  on): at most one card with at most three findings. Quiet: absent when
 *  there is nothing to say or it was dismissed. */
export function LibraryReview({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation()
  const { data: card } = useLibraryReview(enabled)
  const { dismiss, makeVariants } = useLibraryReviewActions()
  const archive = useSetFoodArchived()
  if (!enabled || !card) return null

  async function hide(f: ReviewFinding) {
    await archive.mutateAsync({ id: f.recipes[0].id, archived: true })
    dismiss.mutate(f.key)
  }

  return (
    <section
      className="flex flex-col gap-4 rounded-lg border border-line bg-panel p-4"
      aria-label={t("recipes.review.title")}
      data-testid="library-review"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <h2 className="text-h3 font-semibold text-ink">
            {t("recipes.review.title")}
          </h2>
          <p className="text-small text-ink-muted">
            {t("recipes.review.intro")}
          </p>
        </div>
        <Button
          type="button"
          variant="ghost"
          className="h-11 md:pointer-fine:h-9"
          onClick={() => dismiss.mutate(undefined)}
          data-testid="library-review-dismiss"
        >
          {t("recipes.review.dismiss_card")}
        </Button>
      </div>
      <ul className="flex flex-col gap-4">
        {card.findings.map((f) => (
          <li
            key={f.key}
            className="flex flex-col gap-2 border-t border-line pt-3"
            data-testid={`review-finding-${f.kind}`}
          >
            <FindingText finding={f} />
            <div className="flex flex-wrap gap-2">
              {f.kind === "duplicates" && (
                <Button
                  type="button"
                  variant="outline"
                  className="h-11 md:pointer-fine:h-9"
                  disabled={makeVariants.isPending}
                  onClick={() => makeVariants.mutate(f.key)}
                >
                  {t("recipes.review.make_variants")}
                </Button>
              )}
              {f.kind === "replaced" && (
                <Button
                  type="button"
                  variant="outline"
                  className="h-11 md:pointer-fine:h-9"
                  disabled={archive.isPending}
                  onClick={() => void hide(f).catch(() => undefined)}
                >
                  {t("recipes.review.hide")}
                </Button>
              )}
              <Button
                type="button"
                variant="ghost"
                className="h-11 md:pointer-fine:h-9"
                onClick={() => dismiss.mutate(f.key)}
                data-testid={`review-finding-dismiss-${f.kind}`}
              >
                {t("recipes.review.dismiss_finding")}
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}

function RecipeLink({ recipe }: { recipe: ReviewRecipe }) {
  return (
    <Link
      to="/recipes/$id"
      params={{ id: String(recipe.id) }}
      className="font-semibold text-ink underline underline-offset-2"
    >
      {recipe.name}
    </Link>
  )
}

function FindingText({ finding }: { finding: ReviewFinding }) {
  const { t } = useTranslation()
  const [a, b] = finding.recipes
  switch (finding.kind) {
    case "duplicates":
      return (
        <p className="text-body text-ink">
          <RecipeLink recipe={a} /> {t("recipes.review.and")}{" "}
          <RecipeLink recipe={b} /> {t("recipes.review.look_alike")}
        </p>
      )
    case "replaced":
      return (
        <p className="text-body text-ink">
          <RecipeLink recipe={a} />{" "}
          {t("recipes.review.replaced", { count: finding.count ?? 0 })}
        </p>
      )
    case "data":
      return (
        <div className="flex flex-col gap-1">
          <p className="text-body text-ink">
            {t("recipes.review.data", { count: finding.count ?? 0 })}
          </p>
          <ul className="flex flex-col gap-1">
            {finding.recipes.map((r) => (
              <li key={r.id} className="text-small text-ink-muted">
                <Link
                  to="/recipes/$id/edit"
                  params={{ id: String(r.id) }}
                  className="text-ink underline underline-offset-2"
                >
                  {r.name}
                </Link>
                {": "}
                {(r.missing ?? [])
                  .map((m) => t(`recipes.review.missing_${m}`))
                  .join(", ")}
              </li>
            ))}
          </ul>
        </div>
      )
  }
}
