import { Fragment } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { TriangleAlert } from "lucide-react"

import type { ScaledIngredient } from "@/lib/api/foods"

/** Says a recipe's diet class is unknown (D28) and links each ingredient
 *  the classifier couldn't place to where its class is set. */
export function DietUnknownWarning({
  unknown,
}: {
  /** The recipe's ingredients whose class is unknown. */
  unknown: readonly Pick<ScaledIngredient, "child_id" | "name" | "kind">[]
}) {
  const { t } = useTranslation()
  return (
    <div
      role="status"
      data-testid="diet-unknown-warning"
      className="flex gap-3 rounded-lg border border-line bg-panel px-4 py-3 text-sm text-ink"
    >
      <TriangleAlert
        className="mt-0.5 size-4 shrink-0 text-warning"
        strokeWidth={1.5}
        aria-hidden
      />
      <div className="flex flex-col gap-1">
        <p className="font-semibold">{t("diet.unknown_title")}</p>
        {unknown.length > 0 && (
          <p className="text-ink-muted">
            {t("diet.unknown_body", { count: unknown.length })}{" "}
            {unknown.map((ing, i) => (
              <Fragment key={ing.child_id}>
                {i > 0 && ", "}
                <Link
                  to={
                    ing.kind === "leaf"
                      ? "/recipes/ingredients/$id/edit"
                      : "/recipes/$id"
                  }
                  params={{ id: String(ing.child_id) }}
                  className="text-accent underline underline-offset-2 hover:no-underline"
                >
                  {ing.name}
                </Link>
              </Fragment>
            ))}
          </p>
        )}
      </div>
    </div>
  )
}
