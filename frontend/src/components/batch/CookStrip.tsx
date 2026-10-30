import { useTranslation } from "react-i18next"

import type { ColouredBatch } from "@/lib/plan/batch-colours"
import { weekdayShort } from "@/lib/plan/format"

import { BatchBadge } from "./BatchBadge"
import { useBatchHighlight } from "./batch-highlight"

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
          // Unconsumed frozen portions are planned for the freezer; the rest
          // of the surplus is spare.
          const toFreeze = Math.max(
            0,
            session.frozen_portions - session.frozen_consumed
          )
          const spare = Math.max(0, session.surplus - toFreeze)
          return (
            <li key={session.id} className="shrink-0">
              <button
                type="button"
                onClick={() => reveal(session.id)}
                className="flex min-h-11 items-center gap-1 rounded-lg px-1"
              >
                <BatchBadge
                  sessionId={session.id}
                  colour={colour}
                  variant="cook"
                  portions={session.portions}
                  strip={{
                    weekday: weekdayShort(session.cook_date, i18n),
                    name: session.food_name,
                  }}
                />
                {spare > 0 && (
                  <BatchBadge
                    sessionId={session.id}
                    colour={colour}
                    variant="surplus"
                    count={spare}
                  />
                )}
                {toFreeze > 0 && (
                  <BatchBadge
                    sessionId={session.id}
                    colour={colour}
                    variant="toFreeze"
                    count={toFreeze}
                  />
                )}
              </button>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
