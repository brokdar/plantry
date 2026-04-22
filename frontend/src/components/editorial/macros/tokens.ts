export type MacroKind = "protein" | "carbs" | "fat" | "fiber"

export const MACRO_KCAL_PER_G: Record<MacroKind, number> = {
  protein: 4,
  carbs: 4,
  fat: 9,
  fiber: 2,
}

export const MACRO_DOT_CLASS: Record<MacroKind, string> = {
  protein: "bg-macro-protein",
  carbs: "bg-macro-carbs",
  fat: "bg-macro-fat",
  fiber: "bg-macro-fiber",
}

export const MACRO_TEXT_CLASS: Record<MacroKind, string> = {
  protein: "text-macro-protein",
  carbs: "text-macro-carbs",
  fat: "text-macro-fat",
  fiber: "text-macro-fiber",
}

export const MACRO_SOFT_CLASS: Record<MacroKind, string> = {
  protein: "bg-macro-protein-soft",
  carbs: "bg-macro-carbs-soft",
  fat: "bg-macro-fat-soft",
  fiber: "bg-macro-fiber-soft",
}

export const MACRO_ORDER: MacroKind[] = ["protein", "carbs", "fat"]

export function formatGrams(
  value: number | null | undefined,
  opts: { precision?: number } = {}
): string {
  if (value == null) return "—"
  const precision = opts.precision ?? (value < 10 ? 1 : 0)
  return value.toFixed(precision).replace(/\.0$/, "")
}
