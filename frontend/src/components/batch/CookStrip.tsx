import { useTranslation } from "react-i18next"

import type { Session } from "@/lib/api/sessions"
import type { ColouredBatch } from "@/lib/plan/batch-colours"
import {
  batchBadgeText,
  weekdayShort,
  type BatchBadgeContent,
} from "@/lib/plan/format"

import { BatchBadge } from "./BatchBadge"
import { useBatchHighlight } from "./batch-highlight"

/** The badges of one strip entry: the cook badge, then the dashed extras.
 *  `surplus` (portions − all consumption, never below 0) is split into the
 *  part planned for the freezer and the spare rest. */
function stripBadges(session: Session, weekday: string): BatchBadgeContent[] {
  const toFreeze = Math.min(
    session.surplus,
    Math.max(0, session.frozen_portions - session.frozen_consumed)
  )
  const spare = session.surplus - toFreeze
  return [
    {
      variant: "cook",
      portions: session.portions,
      strip: { weekday, name: session.food_name },
    },
    ...(spare > 0 ? [{ variant: "surplus" as const, count: spare }] : []),
    ...(toFreeze > 0
      ? [{ variant: "toFreeze" as const, count: toFreeze }]
      : []),
  ]
}

/**
 * The cook strip above the board (UX §4, §5.5): one badge per batch in cook
 * order ("Sun · Chili × 6"), followed by dashed "2 spare" / "2 to freeze"
 * badges. Tapping a batch scrolls to its cook cell and highlights it. On
 * phone the row scrolls sideways.
 */
export function CookStrip({ batches }: { batches: readonly ColouredBatch[] }) {
  const { t, i18n } = useTranslation()
  const { reveal } = useBatchHighlight()

  if (batches.length === 0) return null

  return (
    <section aria-label={t("batch.cook_days")}>
      {/* p-1 keeps the 2px + 2px focus ring inside the scroll box. */}
      <ul className="flex gap-1 overflow-x-auto overscroll-x-contain p-1">
        {batches.map(({ session, colour }) => {
          const badges = stripBadges(
            session,
            weekdayShort(session.cook_date, i18n)
          )
          return (
            <li key={session.id} className="shrink-0">
              <button
                type="button"
                aria-label={badges
                  .map((badge) => batchBadgeText(badge, i18n))
                  .join(", ")}
                onClick={() => reveal(session.id, colour)}
                className="flex min-h-11 items-center gap-1 rounded-lg px-1"
              >
                {badges.map((badge) => (
                  <BatchBadge
                    key={badge.variant}
                    sessionId={session.id}
                    colour={colour}
                    {...badge}
                  />
                ))}
              </button>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
