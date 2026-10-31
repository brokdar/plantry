import { Check } from "lucide-react"
import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import { Button } from "@/components/ui/button"
import { BATCH_CLASSES } from "@/lib/plan/batch-colours"
import { formatAmount, formatNumber } from "@/lib/plan/format"
import { plateMacros } from "@/lib/plan/plate-macros"
import type { TodayCookCard, TodayLeftoverCard } from "@/lib/plan/today"
import { useFoodMacros } from "@/lib/queries/foods"
import { cn } from "@/lib/utils"

/**
 * A batch cooked today (UX §5.2): on the batch tint, the cook badge, the
 * recipe, what it covers, the time estimate and **Start cooking**.
 */
export function CookCard({ card }: { card: TodayCookCard }) {
  const { t, i18n } = useTranslation()
  const { session, colour } = card
  return (
    <article
      aria-labelledby={`today-cook-${session.id}`}
      className={cn(
        "flex flex-col gap-3 rounded-lg border border-line p-4",
        BATCH_CLASSES[colour].tint
      )}
      data-testid="today-cook-card"
    >
      <div className="flex flex-wrap items-center gap-2">
        <BatchBadge
          variant="cook"
          portions={session.portions}
          sessionId={session.id}
          colour={colour}
        />
        <span className="text-small text-ink-muted">{card.slotName}</span>
      </div>
      <div className="flex flex-col gap-1">
        <h2
          id={`today-cook-${session.id}`}
          className="font-heading text-h2 font-semibold text-ink"
        >
          {session.food_name}
        </h2>
        <p className="text-small text-ink-muted">{card.coverage}</p>
        {card.minutes !== null && (
          <p className="text-small text-ink-muted tabular-nums">
            {t("today.cook.minutes", {
              minutes: formatNumber(card.minutes, i18n),
            })}
          </p>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        {/* Cook mode (`/cook/$sessionId`) arrives with F6; until then a
            plain link, since a typed Link needs the route to exist. */}
        <Button asChild className="h-11 px-4 md:pointer-fine:h-9">
          <a href={`/cook/${session.id}`} data-testid="today-start-cooking">
            {t("today.cook.start")}
          </a>
        </Button>
        {session.cooked_at && (
          <span className="inline-flex items-center gap-1 text-small font-semibold text-success">
            <Check className="size-4" strokeWidth={1.5} aria-hidden />
            {t("today.cook.cooked")}
          </span>
        )}
      </div>
    </article>
  )
}

/**
 * The day's first leftover, as the main card on a day without cooking: the
 * badge with its use-by day, the meal, and "Reheat 2 portions · 520 kcal
 * each" (kcal for one eater, as on the board).
 */
export function LeftoverCard({ card }: { card: TodayLeftoverCard }) {
  const { t, i18n } = useTranslation()
  const { cell, badge } = card
  const components = cell.plate?.components ?? []
  const macros = useFoodMacros([...new Set(components.map((c) => c.food_id))])
  const kcal = macros.data
    ? formatNumber(plateMacros(components, macros.data.foods).kcal, i18n)
    : null
  const reheat = t("today.leftover.reheat", {
    count: card.portions,
    portions: formatAmount(card.portions, i18n),
  })
  const energy =
    kcal === null
      ? null
      : card.portions === 1
        ? `${kcal} ${t("units.kcal")}`
        : t("today.leftover.kcal_each", { kcal })

  return (
    <article
      aria-labelledby="today-leftover"
      className="flex flex-col gap-3 rounded-lg border border-line bg-panel p-4"
      data-testid="today-leftover-card"
    >
      <div className="flex flex-wrap items-center gap-2">
        <BatchBadge {...badge} suffix={card.useBy ?? undefined} />
        <span className="text-small text-ink-muted">{card.slotName}</span>
      </div>
      <div className="flex flex-col gap-1">
        <h2
          id="today-leftover"
          className="font-heading text-h2 font-semibold text-ink"
        >
          {t("today.leftover.title", { name: cell.mainName })}
        </h2>
        {cell.sideText && (
          <p className="text-small text-ink-muted">{cell.sideText}</p>
        )}
        <p className="text-body text-ink tabular-nums">
          {energy ? `${reheat} · ${energy}` : reheat}
        </p>
      </div>
    </article>
  )
}
