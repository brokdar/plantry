import { use } from "react"
import { useTranslation } from "react-i18next"

import type { ColouredBatch } from "@/lib/plan/batch-colours"
import type { BoardDay } from "@/lib/plan/board"
import { batchBadgeText, weekdayShort } from "@/lib/plan/format"
import { cn } from "@/lib/utils"

import { HighlightedContext, useBatchHighlight } from "./batch-highlight"
import { stripBadges } from "./CookStrip"

/** One lane's rem height; the board reserves `lanes × LANE_REM` under the
 *  day headers. */
export const LANE_REM = 1.5

/** Day columns (0-based) where `sessionId` is eaten, and the one where it is
 *  cooked (-1 when the cook cell is outside the window). */
function laneDays(days: readonly BoardDay[], sessionId: number) {
  const eaten: number[] = []
  let cook = -1
  days.forEach((day, i) => {
    if (day.cells.some((c) => c.sessionIds.includes(sessionId))) eaten.push(i)
    if (day.cells.some((c) => c.cookOf.includes(sessionId))) cook = i
  })
  return { eaten, cook }
}

interface BatchLanesProps {
  batches: readonly ColouredBatch[]
  days: readonly BoardDay[]
  /** The board grid's column template, so lanes line up with the days. */
  columns: string
}

/**
 * The batch line strip (the board's signature): one lane per batch under the
 * day headers. A 3 px line runs from the cook day to the last leftover day
 * with a filled dot on the cook day and hollow dots on the leftovers; the
 * label sits in the meal column. Each lane is a button in a labelled list:
 * it names the batch like the cook strip does, brings the batch forward on
 * hover and focus (the linked highlight) and reveals the cook cell on
 * activation. Positioned by the board over the space it reserves.
 */
export function BatchLanes({ batches, days, columns }: BatchLanesProps) {
  const { t, i18n } = useTranslation()
  const { highlight, reveal } = useBatchHighlight()
  const highlighted = use(HighlightedContext)

  if (batches.length === 0) return null

  return (
    <section
      aria-label={t("batch.cook_days")}
      className="pointer-events-none absolute inset-x-0 top-[4rem] z-[5]"
    >
      <ul
        data-highlight-active={highlighted ? "" : undefined}
        className={cn("grid px-1", columns)}
        style={{ gridAutoRows: `${LANE_REM}rem` }}
      >
        {batches.map(({ session, colour }) => {
          const badges = stripBadges(
            session,
            weekdayShort(session.cook_date, i18n)
          )
          const label = badges
            .map((badge) => batchBadgeText(badge, i18n))
            .join(", ")
          const { eaten, cook } = laneDays(days, session.id)
          const start = eaten.length
            ? Math.min(...eaten, cook < 0 ? 99 : cook)
            : -1
          const end = eaten.length ? Math.max(...eaten, cook) : -1
          const span = end - start + 1
          const active = highlighted?.sessionId === session.id
          return (
            <li
              key={session.id}
              data-highlighted={active ? colour : undefined}
              style={
                { "--lane": `var(--batch-${colour})` } as React.CSSProperties
              }
              className={cn(
                "pointer-events-auto col-span-full grid grid-cols-subgrid transition-opacity duration-200 ease-out",
                "[ul[data-highlight-active]_&:not([data-highlighted])]:opacity-30"
              )}
            >
              <button
                type="button"
                aria-label={label}
                onPointerEnter={() => highlight(session.id, colour)}
                onFocus={() => highlight(session.id, colour)}
                onClick={() => reveal(session.id, colour)}
                className="group/lane col-span-full grid grid-cols-subgrid items-center rounded-md"
              >
                <span
                  aria-hidden="true"
                  title={session.food_name}
                  className="col-start-1 min-w-0 truncate pr-1.5 text-left text-caption font-semibold text-(--lane) group-hover/lane:underline"
                >
                  {session.food_name}
                </span>
                {badges.length > 1 && (
                  <span className="sr-only">
                    {badges
                      .slice(1)
                      .map((badge) => batchBadgeText(badge, i18n))
                      .join(", ")}
                  </span>
                )}
                {span > 0 && (
                  <span
                    aria-hidden="true"
                    className="relative grid grid-cols-subgrid items-center"
                    style={{
                      gridColumn: `${start + 2} / span ${span}`,
                      gridTemplateRows: "1fr",
                    }}
                  >
                    <span
                      className="absolute top-1/2 h-[3px] -translate-y-1/2 rounded-full bg-(--lane) transition-[height] duration-200 ease-out group-hover/lane:h-1 group-focus-visible/lane:h-1"
                      style={{
                        left: `calc((100% - ${(span - 1) * 4}px) / ${2 * span})`,
                        right: `calc((100% - ${(span - 1) * 4}px) / ${2 * span})`,
                      }}
                    />
                    {eaten.map((i) => (
                      <span
                        key={i}
                        className={cn(
                          "relative z-10 justify-self-center rounded-full",
                          i === cook
                            ? "size-3 bg-(--lane)"
                            : "size-[11px] border-[2.5px] border-(--lane) bg-panel"
                        )}
                        style={{ gridColumn: i - start + 1, gridRow: 1 }}
                      />
                    ))}
                  </span>
                )}
              </button>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
