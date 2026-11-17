import { ArrowLeft } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { AlternativeConstraints, AlternativeWhat } from "@/lib/api/meal"
import { KNOWN_ISSUE_CODES, type Plan } from "@/lib/api/plan"
import { formatNumber } from "@/lib/plan/format"
import { lighterKcal } from "@/lib/plan/meal-actions"
import {
  useAlternatives,
  useApplyAlternative,
  type DescribeAlsoChanged,
} from "@/lib/queries/meal"
import { cn } from "@/lib/utils"

export interface AlternativesTarget {
  what: AlternativeWhat
  componentId?: number
  /** The list's heading ("Something else", "Change main", …). */
  title: string
}

type Chip = "quicker" | "vegetarian" | "protein" | "lighter"
const CHIPS: Chip[] = ["quicker", "vegetarian", "protein", "lighter"]

/** Quicker: at most 30 minutes; more protein: at least 25 g per eater. */
const QUICK_MINUTES = 30
const MORE_PROTEIN_G = 25

/** The chips' constraints for `POST /api/plan/alternatives` (exported for
 *  its test). */
export function chipConstraints(
  chips: Set<Chip>,
  lighter: number | undefined
): AlternativeConstraints | undefined {
  const c: AlternativeConstraints = {}
  if (chips.has("quicker")) c.max_minutes = QUICK_MINUTES
  if (chips.has("vegetarian")) c.diet = "vegetarian"
  if (chips.has("protein")) c.min_protein_g = MORE_PROTEIN_G
  if (chips.has("lighter") && lighter !== undefined) c.max_kcal = lighter
  return Object.keys(c).length ? c : undefined
}

/**
 * The alternatives list in the plate sheet (meal editing 3a): ranked rows
 * from the code with minutes, kcal, the effect on the day, a badge when it
 * adds a cooking or eats an existing batch, and the new issues; filter
 * chips set the constraints. Choosing a row applies it as one action with
 * one Undo and closes the sheet.
 */
export function AlternativesPanel({
  plan,
  plateId,
  date,
  target,
  describe,
  onBack,
  onDone,
}: {
  plan: Plan
  plateId: number
  date: string
  target: AlternativesTarget
  describe: DescribeAlsoChanged
  onBack: () => void
  onDone: () => void
}) {
  const { t, i18n } = useTranslation()
  const [chips, setChips] = useState<Set<Chip>>(new Set())
  const lighter = lighterKcal(plan, date)
  const input = {
    plate_id: plateId,
    what: target.what,
    plate_component_id: target.componentId,
    constraints: chipConstraints(chips, lighter),
  }
  const list = useAlternatives(plan.window.start, input)
  const apply = useApplyAlternative(describe)

  function toggle(c: Chip) {
    setChips((prev) => {
      const next = new Set(prev)
      if (next.has(c)) next.delete(c)
      else next.add(c)
      return next
    })
  }

  return (
    <section
      className="flex flex-col gap-3"
      aria-labelledby="alt-title"
      data-testid="alternatives"
    >
      <div className="flex items-center gap-1">
        <Button
          type="button"
          variant="ghost"
          className="size-11 md:pointer-fine:size-8"
          aria-label={t("common.back")}
          onClick={onBack}
        >
          <ArrowLeft strokeWidth={1.5} aria-hidden />
        </Button>
        <h3
          id="alt-title"
          className="font-sans text-body font-semibold text-ink"
        >
          {target.title}
        </h3>
      </div>
      <div
        className="flex flex-wrap gap-2"
        role="group"
        aria-label={t("plan.alt.filters")}
      >
        {CHIPS.map((c) => (
          <Button
            key={c}
            type="button"
            size="sm"
            variant={chips.has(c) ? "default" : "outline"}
            aria-pressed={chips.has(c)}
            disabled={
              apply.isPending || (c === "lighter" && lighter === undefined)
            }
            className="h-9 rounded-tag"
            onClick={() => toggle(c)}
          >
            {t(`plan.alt.chip.${c}`)}
          </Button>
        ))}
      </div>
      {list.isPending ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-16 rounded-cell" />
          <Skeleton className="h-16 rounded-cell" />
        </div>
      ) : list.isError ? (
        <p className="text-small text-danger">{t("plan.alt.error")}</p>
      ) : list.data.length === 0 ? (
        <p className="text-small text-ink-muted">{t("plan.alt.none")}</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {list.data.map((alt) => (
            <li key={`${alt.food_id}-${alt.source}-${alt.session_id ?? 0}`}>
              <button
                type="button"
                data-testid="alt-row"
                disabled={apply.isPending}
                onClick={() =>
                  apply.mutate({ plateId, alt }, { onSuccess: onDone })
                }
                className={cn(
                  "flex w-full flex-col gap-1 rounded-cell border border-line bg-panel p-3 text-left",
                  "hover:bg-panel-sunken focus-visible:outline-2 focus-visible:outline-accent disabled:opacity-60"
                )}
              >
                <span className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate font-medium text-ink">
                    {alt.name}
                  </span>
                  {alt.source === "session" ? (
                    <Badge variant="secondary">
                      {t("plan.alt.eats_batch")}
                    </Badge>
                  ) : alt.source === "fresh" ? (
                    <Badge variant="outline">{t("plan.alt.new_cooking")}</Badge>
                  ) : null}
                </span>
                <span className="text-small text-ink-muted tabular-nums">
                  {[
                    alt.source === "fresh" && alt.total_minutes
                      ? t("plan.alt.minutes", { count: alt.total_minutes })
                      : null,
                    `${formatNumber(alt.kcal, i18n)} ${t("units.kcal")}`,
                    alt.day_pct
                      ? t("plan.alt.day_pct", { pct: Math.round(alt.day_pct) })
                      : t("plan.alt.day_kcal", {
                          kcal: formatNumber(alt.day_kcal, i18n),
                        }),
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </span>
                {alt.new_issues?.map((issue, i) => (
                  <span key={i} className="text-small text-warning">
                    {(KNOWN_ISSUE_CODES as readonly string[]).includes(
                      issue.code
                    )
                      ? t(`plan.issue.${issue.code}`, issue.params)
                      : t("plan.issue.other", { rule: issue.rule })}
                  </span>
                ))}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
