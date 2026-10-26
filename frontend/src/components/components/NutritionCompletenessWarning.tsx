import { Fragment } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { TriangleAlert } from "lucide-react"

import type { MissingNutritionLeaf } from "@/lib/api/foods"
import { cn } from "@/lib/utils"

type NutritionCompletenessWarningProps = {
  /** Ingredients without core macros. Renders nothing when empty. */
  missing: readonly MissingNutritionLeaf[]
  /** Open the ingredient links in a new tab, for screens holding unsaved
   *  work (the import review). */
  openInNewTab?: boolean
  className?: string
}

/** Says the recipe's nutrition under-counts and links each ingredient that
 *  lacks data to its editor. Shared by the recipe detail, editor and import
 *  review. */
export function NutritionCompletenessWarning({
  missing,
  openInNewTab = false,
  className,
}: NutritionCompletenessWarningProps) {
  const { t } = useTranslation()
  if (missing.length === 0) return null

  return (
    <div
      role="status"
      data-testid="nutrition-completeness-warning"
      className={cn(
        "flex gap-3 rounded-lg border border-line bg-panel px-4 py-3 text-sm text-ink",
        className
      )}
    >
      <TriangleAlert
        className="mt-0.5 size-4 shrink-0 text-warning"
        strokeWidth={1.5}
        aria-hidden
      />
      <div className="flex flex-col gap-1">
        <p className="font-semibold">{t("nutrition_warning.title")}</p>
        <p className="text-ink-muted">
          {t("nutrition_warning.body", { count: missing.length })}{" "}
          {missing.map((leaf, i) => (
            <Fragment key={leaf.id}>
              {i > 0 && ", "}
              <Link
                to="/ingredients/$id/edit"
                params={{ id: String(leaf.id) }}
                className="text-accent underline underline-offset-2 hover:no-underline"
                target={openInNewTab ? "_blank" : undefined}
                rel={openInNewTab ? "noopener noreferrer" : undefined}
              >
                {leaf.name}
                {openInNewTab && (
                  <span className="sr-only"> {t("common.opens_new_tab")}</span>
                )}
              </Link>
            </Fragment>
          ))}
        </p>
      </div>
    </div>
  )
}
