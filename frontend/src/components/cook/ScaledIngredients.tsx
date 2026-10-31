import { useState } from "react"
import { useTranslation } from "react-i18next"

import type { ScaledIngredient } from "@/lib/api/foods"
import { formatAmount, formatQuantity } from "@/lib/plan/format"
import { cn } from "@/lib/utils"

/**
 * The recipe's ingredients at the cook's portions (UX §5.3), already rounded
 * by the backend (D26). Each row ticks off with one tap; ticks live only as
 * long as cook mode is open. A count unit the recipe uses only in part
 * says what to buy: "1 can, using ⅔".
 */
export function ScaledIngredients({
  ingredients,
}: {
  ingredients: readonly ScaledIngredient[] | undefined
}) {
  const { t, i18n } = useTranslation()
  const [ticked, setTicked] = useState<ReadonlySet<number>>(new Set())

  if (!ingredients) {
    return (
      <div className="flex flex-col gap-3" aria-hidden>
        {[0, 1, 2, 3].map((i) => (
          <div
            key={i}
            className="h-14 w-full animate-pulse rounded-cell bg-panel-sunken"
          />
        ))}
      </div>
    )
  }
  if (ingredients.length === 0) {
    return <p className="text-h3 text-ink-muted">{t("cook.no_ingredients")}</p>
  }

  function toggle(id: number) {
    setTicked((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }

  return (
    <ul
      className="flex flex-col divide-y divide-line"
      data-testid="cook-ingredients"
    >
      {ingredients.map((ing) => {
        const done = ticked.has(ing.child_id)
        const partial =
          ing.buy !== null &&
          (ing.buy.amount !== ing.display.amount ||
            ing.buy.unit !== ing.display.unit)
        return (
          <li key={ing.child_id} data-testid="cook-ingredient">
            <label className="flex min-h-14 cursor-pointer items-center gap-4 py-2">
              <input
                type="checkbox"
                checked={done}
                onChange={() => toggle(ing.child_id)}
                className="size-6 shrink-0 accent-accent dark:scheme-dark"
              />
              <span
                className={cn(
                  "flex-1 text-h3",
                  done ? "text-ink-muted line-through" : "text-ink"
                )}
              >
                {ing.name}
              </span>
              <span className="flex flex-col items-end text-right">
                <span
                  className={cn(
                    "text-h3 font-semibold tabular-nums",
                    done ? "text-ink-muted line-through" : "text-ink"
                  )}
                >
                  {formatQuantity(ing.display.amount, ing.display.unit, i18n)}
                </span>
                {partial && ing.buy && (
                  <span className="text-small text-ink-muted tabular-nums">
                    {t("cook.buy_hint", {
                      buy: formatQuantity(ing.buy.amount, ing.buy.unit, i18n),
                      use: formatAmount(ing.display.amount, i18n),
                    })}
                  </span>
                )}
              </span>
            </label>
          </li>
        )
      })}
    </ul>
  )
}
