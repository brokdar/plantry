import { useTranslation } from "react-i18next"

import type { ShoppingSection } from "@/lib/api/shopping"
import { cn } from "@/lib/utils"

import { ShoppingRow, type UsedInContext } from "./ShoppingRow"

interface SectionListProps {
  /** In walk order, as the API sends them. */
  sections: readonly ShoppingSection[]
  ticked: ReadonlySet<number>
  onToggle: (foodId: number) => void
  usedIn: UsedInContext
  large?: boolean
}

/**
 * The store sections in walk order, two columns on desktop (read down, then
 * across). The split is fixed by item count, so expanding a row never moves
 * a section to the other column. Within a section, ticked rows sink to the
 * bottom and keep their relative order.
 */
export function SectionList({
  sections,
  ticked,
  onToggle,
  usedIn,
  large = false,
}: SectionListProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-x-12 lg:grid-cols-2">
      {splitColumns(sections).map((column, i) => (
        <div key={i} className="flex min-w-0 flex-col">
          {column.map(({ section, items }) => {
            const headingId = `shop-section-${section}`
            const ordered = [
              ...items.filter((item) => !ticked.has(item.food_id)),
              ...items.filter((item) => ticked.has(item.food_id)),
            ]
            return (
              <section
                key={section}
                aria-labelledby={headingId}
                data-testid="shop-section"
                data-section={section}
                className="mb-8"
              >
                <h2
                  id={headingId}
                  className={cn(
                    "flex items-baseline justify-between gap-3 border-b border-ink pb-1 font-semibold text-ink",
                    large ? "text-h2" : "text-h3"
                  )}
                >
                  {t(`shop.section.${section}`)}
                  <span
                    aria-hidden="true"
                    className="font-sans text-small font-normal text-ink-muted tabular-nums"
                  >
                    {items.length}
                  </span>
                </h2>
                <ul>
                  {ordered.map((item) => (
                    <ShoppingRow
                      key={item.food_id}
                      item={item}
                      section={section}
                      ticked={ticked.has(item.food_id)}
                      onToggle={onToggle}
                      usedIn={usedIn}
                      large={large}
                    />
                  ))}
                </ul>
              </section>
            )
          })}
        </div>
      ))}
    </div>
  )
}

/** Splits the sections in two at about half the items, keeping walk
 *  order: the first column takes sections until it holds at least half. */
function splitColumns(
  sections: readonly ShoppingSection[]
): ShoppingSection[][] {
  const total = sections.reduce((n, s) => n + s.items.length, 0)
  const left: ShoppingSection[] = []
  let count = 0
  for (const s of sections) {
    if (count >= total / 2) break
    left.push(s)
    count += s.items.length
  }
  return [left, sections.slice(left.length)]
}
