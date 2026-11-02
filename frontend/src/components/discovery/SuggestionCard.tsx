import { ExternalLink } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import type { Suggestion } from "@/lib/api/discovery"

import { NewLegend } from "./NewLegend"

/** A discovery idea: photo, facts, why, ingredients and the source. */
export function SuggestionCard({ suggestion: s }: { suggestion: Suggestion }) {
  const { t } = useTranslation()
  const hasNew = s.ingredients.some((i) => !i.known)
  const facts = [
    s.total_minutes !== null &&
      t("discovery.card.minutes", { count: s.total_minutes }),
    t("discovery.card.portions", { count: s.reference_portions }),
    s.cuisine,
  ].filter(Boolean)

  return (
    <article className="flex flex-col gap-4" data-testid="suggestion-card">
      <Photo suggestion={s} />
      <header className="flex flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-heading text-h3 text-ink">{s.name}</h3>
          {s.grounded && (
            <span
              className="rounded-full border border-line px-2 py-0.5 text-small text-ink-muted"
              data-testid="suggestion-grounded"
            >
              {t("discovery.card.grounded")}
            </span>
          )}
        </div>
        <p className="text-small text-ink-muted">{facts.join(" · ")}</p>
        {s.nutrition_complete ? (
          <p className="text-small text-ink-muted">
            {t("discovery.card.macros", {
              kcal: Math.round(s.per_portion.kcal),
              protein: Math.round(s.per_portion.protein),
            })}
          </p>
        ) : (
          <p
            className="text-small text-ink-muted"
            data-testid="suggestion-incomplete"
          >
            {t("discovery.card.nutrition_incomplete")}
            {s.missing.length > 0 &&
              ` · ${t("discovery.card.nutrition_missing", { names: s.missing.join(", ") })}`}
          </p>
        )}
      </header>
      {s.why && <p className="text-body text-ink">{s.why}</p>}
      <section
        aria-label={t("discovery.card.ingredients")}
        className="flex flex-col gap-2"
      >
        <h4 className="text-small font-semibold text-ink">
          {t("discovery.card.ingredients")}
        </h4>
        <ul className="flex flex-col gap-1">
          {s.ingredients.map((i, idx) => (
            <li key={idx} className="flex items-baseline gap-2 text-body">
              <span className="text-ink">{i.raw_text}</span>
              {!i.known && (
                <span
                  className="text-small font-semibold text-accent"
                  data-testid="suggestion-new"
                >
                  {t("discovery.card.new")}
                </span>
              )}
            </li>
          ))}
        </ul>
        {hasNew && <NewLegend />}
      </section>
      <a
        href={s.source_url}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center gap-1 text-small text-accent underline-offset-2 hover:underline"
      >
        {t("discovery.card.source")}
        <ExternalLink className="size-3.5" aria-hidden="true" />
      </a>
    </article>
  )
}

/** The 16:9 photo; without one (or when it fails to load) the name set as a
 *  typographic placeholder (UX §5.8). */
function Photo({ suggestion: s }: { suggestion: Suggestion }) {
  const { t } = useTranslation()
  const [failed, setFailed] = useState(false)
  if (s.image_url && !failed) {
    return (
      <img
        src={s.image_url}
        alt={t("discovery.card.photo_alt", { name: s.name })}
        className="aspect-video w-full rounded-lg object-cover"
        onError={() => setFailed(true)}
      />
    )
  }
  return (
    <div
      aria-hidden="true"
      className="flex aspect-video w-full items-center justify-center rounded-lg bg-panel-sunken p-4 text-center font-heading text-h3 text-ink-muted"
      data-testid="suggestion-placeholder"
    >
      {s.name}
    </div>
  )
}
