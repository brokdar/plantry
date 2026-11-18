import type { i18n as I18n, TFunction } from "i18next"

import type { AlsoChanged, EmptyReason, MealCell } from "@/lib/api/meal"
import type { Plan } from "@/lib/api/plan"
import type { Session } from "@/lib/api/sessions"

import { formatDate } from "./format"

// View-model helpers for the meal-level actions (meal editing slice 5).

export interface CellOption extends MealCell {
  /** `date|slot_id`. */
  key: string
  /** The cell's plate, when taken. */
  plateId: number | null
  /** The taken cell's main dish. */
  name: string | null
}

function slotOrder(plan: Plan): Map<number, number> {
  return new Map(plan.slots.map((s) => [s.id, s.sort_order]))
}

function byCell(order: Map<number, number>) {
  return (a: MealCell, b: MealCell) =>
    a.date.localeCompare(b.date) ||
    (order.get(a.slot_id) ?? 0) - (order.get(b.slot_id) ?? 0)
}

/**
 * The week's cells a plate can go to with Move to… / Copy to…: plan cells
 * (not No plate, not routine) from `today` on, without `except`, in board
 * order. `taken: false` keeps only free cells.
 */
export function targetCells(
  plan: Plan,
  today: string,
  { except, taken = false }: { except?: string; taken?: boolean } = {}
): CellOption[] {
  return plan.cells
    .filter(
      (c) =>
        c.mode === "plan" &&
        c.date >= today &&
        `${c.date}|${c.slot_id}` !== except &&
        (taken || c.plate === null)
    )
    .map((c) => ({
      key: `${c.date}|${c.slot_id}`,
      date: c.date,
      slot_id: c.slot_id,
      plateId: c.plate?.id ?? null,
      name: c.plate ? mainName(c.plate.components) : null,
    }))
    .sort(byCell(slotOrder(plan)))
}

function mainName(components: { food_name: string; sort_order: number }[]) {
  const first = [...components].sort((a, b) => a.sort_order - b.sort_order)[0]
  return first?.food_name ?? null
}

/**
 * The cells a batch's cooking may move to: plan cells from today on, on or
 * before its first meal that is not frozen and no later than any meal's
 * keeping time allows (each meal within `keep_days` of the cook). The
 * backend checks again and refuses with its reasons.
 */
export function cookCells(
  plan: Plan,
  session: Session,
  today: string
): CellOption[] {
  const order = slotOrder(plan)
  const cmp = byCell(order)
  const meals = session.links
    .filter((l) => l.kind !== "frozen")
    .map((l) => ({ date: l.date, slot_id: l.slot_id }))
    .sort(cmp)
  const first = meals[0]
  const last = meals.at(-1)
  const current = `${session.cook_date}|${session.cook_slot_id}`
  return targetCells(plan, today, { taken: true, except: current }).filter(
    (c) =>
      (!first || cmp(c, first) <= 0) &&
      (!last || daysBetween(c.date, last.date) <= session.keep_days)
  )
}

function daysBetween(a: string, b: string): number {
  return Math.round((Date.parse(b) - Date.parse(a)) / 86_400_000)
}

/**
 * "Lighter": the cell's share of the day's kcal target (per eater), the
 * day target spread evenly over the day's plated meals; undefined without
 * a target.
 */
export function lighterKcal(plan: Plan, date: string): number | undefined {
  const target =
    plan.days.find((d) => d.date === date)?.target_kcal ?? plan.targets?.kcal
  if (!target) return undefined
  const meals = plan.cells.filter((c) => c.date === date && c.plate).length
  return Math.round(target / Math.max(1, meals))
}

const DROP_RULES = ["H2", "H3", "H4", "H5", "H8", "H9", "nutrition_missing"]

/**
 * Why an alternatives list is empty, in plain words: the dishes that fit the
 * position and filters, and what each rule dropped, the most common first
 * ("would add a cooking over your limit (5), eaten recently (2)"). `dropped`
 * keys are `rule.code`, or the bare `nutrition_missing` (a dish with incomplete nutrition data); a rule without copy shows its name. With no
 * candidate the filters are the reason.
 */
export function emptyReasonText(
  reason: EmptyReason | undefined,
  t: TFunction
): string {
  if (!reason || reason.candidates === 0) return t("plan.alt.none")
  const byRule = new Map<string, number>()
  for (const [key, n] of Object.entries(reason.dropped ?? {})) {
    const rule = key.split(".")[0]
    byRule.set(rule, (byRule.get(rule) ?? 0) + n)
  }
  const reasons = [...byRule]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([rule, count]) =>
      t("plan.alt.dropped.entry", {
        why: DROP_RULES.includes(rule)
          ? t(`plan.alt.dropped.${rule}`)
          : t("plan.alt.dropped.other", { rule }),
        count,
      })
    )
  if (reasons.length === 0) return t("plan.alt.none")
  return t("plan.alt.none_but", {
    candidates: reason.candidates,
    reasons: reasons.join(", "),
  })
}

/** "Thu dinner" for a cell. */
export function cellLabel(
  cell: MealCell,
  t: TFunction,
  i18n: I18n,
  slotName: (id: number) => string
): string {
  return t("plan.cell.name", {
    weekday: formatDate(cell.date, i18n, { weekday: "short" }),
    slot: slotName(cell.slot_id),
  })
}

/**
 * The plain words for what else an edit changes, one line per entry:
 * "Lasagne will now be cooked Thu lunch", "Fri dinner will cook fresh".
 */
export function alsoChangedLines(
  list: AlsoChanged[],
  plan: Plan,
  t: TFunction,
  i18n: I18n,
  slotName: (id: number) => string
): string[] {
  const sessions = new Map(plan.sessions.map((s) => [s.id, s]))
  return list.map((a) => {
    const name = sessions.get(a.session_id)?.food_name ?? ""
    const cell = cellLabel(a, t, i18n, slotName)
    switch (a.what) {
      case "cook_moved":
      case "became_cooking_meal":
        return t("plan.meal.also.cook_moved", { name, cell })
      case "cooks_fresh":
        return t("plan.meal.also.cooks_fresh", { name, cell })
      case "batch_resized":
        return t("plan.meal.also.batch_resized", { name })
    }
    return ""
  })
}
