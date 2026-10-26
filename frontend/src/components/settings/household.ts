export const ADULT_PORTION = 1
export const CHILD_PORTION = 0.5
export const MAX_HOUSEHOLD_PORTIONS = 20

/** Portions for a household: adults × 1 + children × 0.5. */
export function householdPortions(adults: number, children: number): number {
  return adults * ADULT_PORTION + children * CHILD_PORTION
}

/** Splits stored portions back into adults and children (whole adults,
 *  the remainder in children). Only `household_portions` is stored. */
export function householdCounts(portions: number): {
  adults: number
  children: number
} {
  const adults = Math.floor(portions / ADULT_PORTION)
  const children = Math.round((portions - adults) / CHILD_PORTION)
  return { adults, children }
}

/** "1.5" in English, "1,5" in German. */
export function formatPortions(portions: number, locale: string): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(
    portions
  )
}
