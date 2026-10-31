import type { ShoppingListItem, ShoppingSection } from "@/lib/api/shopping"
import { formatQuantity, type Formatter } from "@/lib/plan/format"

/** What to buy, as shown on a row: the shopping unit (`buy`, count units)
 *  when present, otherwise the display quantity ("3 pcs", "600 g"). */
export function quantityText(item: ShoppingListItem, f: Formatter): string {
  const q = item.buy ?? item.display
  return formatQuantity(q.amount, q.unit, f)
}

/**
 * The list as plain text for the clipboard: each section's name, then one
 * "- Onion, 3 pcs" line per item still to buy. Ticked items and sections
 * left empty are dropped; sections are separated by a blank line.
 */
export function shoppingText(
  sections: readonly ShoppingSection[],
  ticked: ReadonlySet<number>,
  f: Formatter
): string {
  return sections
    .map((section) => {
      const lines = section.items
        .filter((item) => !ticked.has(item.food_id))
        .map((item) =>
          f.t("shop.copy_line", {
            name: item.name,
            quantity: quantityText(item, f),
          })
        )
      if (lines.length === 0) return null
      return [f.t(`shop.section.${section.section}`), ...lines].join("\n")
    })
    .filter((block) => block !== null)
    .join("\n\n")
}
