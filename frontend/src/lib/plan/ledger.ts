import type {
  PlanDay,
  PlanTargets,
  SoftIssue,
  WeekLedger,
} from "@/lib/api/plan"

import { formatDate, formatNumber, type Formatter } from "./format"

/** `ok` is quiet; `low` and `high` are the highlighted, out-of-range states;
 *  `info` is a value without a range to judge it by. */
export type LedgerStatus = "ok" | "low" | "high" | "info"

export interface LedgerItem {
  key: "kcal" | "protein" | "carbs" | "fat" | "fiber"
  label: string
  value: string
  detail: string
  status: LedgerStatus
  /** The status in words (no colour-only meaning); null for `info`. */
  statusText: string | null
}

export interface Ledger {
  items: LedgerItem[]
  /** The DGE rules that are off this week, one line each; empty while all
   *  hold. */
  dge: string[]
  /** Hints on single days (kcal outside the band, protein, a light meal
   *  that is not lighter). */
  dayHints: string[]
}

const has = (soft: SoftIssue[], code: string) =>
  soft.some((s) => s.code === code)
const num = (v: string | number | undefined) => Number(v ?? 0)

function statusText(status: LedgerStatus, f: Formatter): string | null {
  if (status === "info") return null
  return f.t(`plan.ledger.status.${status}`)
}

/** Whether a share outside its range sits below or above it. */
function sideOf(pct: number, soft: SoftIssue[], code: string): LedgerStatus {
  const issue = soft.find((s) => s.code === code)
  if (!issue) return "ok"
  return pct < num(issue.params.range_low_pct) ? "low" : "high"
}

/**
 * The week ledger's view model: the compact nutrition line items with their
 * state, the DGE rules that are off and the hints on single days. Every
 * state comes from the soft issues the server judged (the policy's bands are
 * not repeated here), so the strip and the checker cannot disagree.
 */
export function buildLedger(
  week: WeekLedger,
  soft: SoftIssue[],
  f: Formatter
): Ledger {
  const n = (v: number, digits = 0) => formatNumber(v, f, digits)
  const items: LedgerItem[] = []

  const kcalStatus: LedgerStatus = has(soft, "week_kcal_below_band")
    ? "low"
    : has(soft, "week_kcal_above_band")
      ? "high"
      : "ok"
  items.push({
    key: "kcal",
    label: f.t("plan.ledger.kcal"),
    value: f.t("plan.ledger.pct", { pct: n(week.pct) }),
    detail: f.t(
      week.normal_target_kcal !== undefined
        ? "plan.ledger.kcal_detail_light"
        : "plan.ledger.kcal_detail",
      { avg: n(week.avg_kcal), target: n(week.target_kcal) }
    ),
    status: kcalStatus,
    statusText: statusText(kcalStatus, f),
  })

  const perKg = (week.target_protein_g ?? 0) > 0
  const proteinStatus: LedgerStatus = perKg
    ? has(soft, "week_protein_below_target")
      ? "low"
      : "ok"
    : "info"
  items.push({
    key: "protein",
    label: f.t("plan.ledger.protein"),
    value: perKg
      ? f.t("plan.ledger.pct", { pct: n(week.protein_pct ?? 0) })
      : f.t("plan.ledger.protein_avg", { g: n(week.avg_protein_g) }),
    detail: perKg
      ? f.t("plan.ledger.protein_detail", {
          avg: n(week.avg_protein_g),
          target: n(week.target_protein_g ?? 0),
        })
      : "",
    status: proteinStatus,
    statusText: statusText(proteinStatus, f),
  })

  for (const [key, pct, code] of [
    ["carbs", week.carbs_pct, "week_carbs_out_of_range"],
    ["fat", week.fat_pct, "week_fat_out_of_range"],
  ] as const) {
    const status = sideOf(pct, soft, code)
    items.push({
      key,
      label: f.t(`plan.ledger.${key}`),
      value: f.t("plan.ledger.pct", { pct: n(pct) }),
      detail: f.t("plan.ledger.of_energy"),
      status,
      statusText: statusText(status, f),
    })
  }

  if (week.fiber_g) {
    items.push({
      key: "fiber",
      label: f.t("plan.ledger.fiber"),
      value: f.t("plan.ledger.grams", { g: n(week.fiber_g) }),
      detail: f.t("plan.ledger.per_day"),
      status: "info",
      statusText: null,
    })
  }

  const dge = soft
    .filter((s) => s.rule === "S7")
    .map((s) =>
      f.t(`plan.soft.${s.code}`, {
        meat: n(num(s.params.meat_g)),
        max: n(num(s.params.max_g)),
      })
    )

  const dayHints = soft
    .filter((s) => s.date !== undefined && s.rule !== "S7")
    .map((s) => dayHint(s, f))

  return { items, dge, dayHints }
}

function dayHint(s: SoftIssue, f: Formatter): string {
  const day = formatDate(s.date!, f, { weekday: "short", day: "numeric" })
  const p = s.params
  const n = (v: string | number) => formatNumber(num(v), f)
  let code = s.code
  // The protein day hint has two bases: grams per kg or a share.
  if (code === "protein_below_target" && p.target_g === undefined) {
    code = "protein_below_share"
  }
  return f.t(`plan.soft.${code}`, {
    day,
    kcal: n(p.kcal),
    low: n(p.band_low_kcal),
    high: n(p.band_high_kcal),
    protein: n(p.protein_g),
    target: n(p.target_g),
    pct: n(p.protein_pct),
    target_pct: n(p.target_pct),
    max: n(p.max_kcal),
  })
}

/** The targets a day's meter measures against: a light day asks for its
 *  adjusted kcal target, every other day for the profile's. */
export function dayMeterTargets(
  targets: PlanTargets | null,
  day: Pick<PlanDay, "light" | "target_kcal">
): PlanTargets | null {
  if (!targets || day.light !== "day" || !day.target_kcal) return targets
  return { ...targets, kcal: day.target_kcal }
}

/** The label of a day's light mark ("Light day", "Light: Dinner", "Hearty:
 *  Dinner" for a hearty request); null on
 *  a day without a light request. `slotName` resolves "dinner" to the
 *  slot's display name. */
export function lightLabel(
  day: Pick<PlanDay, "light" | "light_slot" | "light_level">,
  slotName: (short: string) => string,
  f: Formatter
): string | null {
  const hearty = day.light_level === "hearty"
  if (day.light === "day") {
    return hearty ? f.t("plan.hearty.day") : f.t("plan.light.day")
  }
  if (day.light === "meal") {
    const slot = slotName(day.light_slot ?? "")
    return hearty
      ? f.t("plan.hearty.meal", { slot })
      : f.t("plan.light.meal", { slot })
  }
  return null
}
