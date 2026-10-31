import type { Plan } from "@/lib/api/plan"
import type { Session } from "@/lib/api/sessions"
import type { Today, TodayVerdict } from "@/lib/api/today"
import { colourBatches, type BatchColour } from "@/lib/plan/batch-colours"
import {
  buildBoard,
  type Board,
  type BoardCell,
  type CellBadge,
} from "@/lib/plan/board"
import {
  coverageText,
  formatAmount,
  formatDate,
  type Formatter,
} from "@/lib/plan/format"
import { slotLabel } from "@/lib/slot-label"

/** A batch cooked on the day: the cook card (UX §5.2). */
export interface TodayCookCard {
  session: Session
  colour: BatchColour
  slotName: string
  /** "Covers Sun dinner, Mon lunch, Mon dinner". */
  coverage: string
  minutes: number | null
}

/** The first leftover of the day, shown as the main card on a day without
 *  cooking. */
export interface TodayLeftoverCard {
  cell: BoardCell
  badge: CellBadge
  slotName: string
  /** "use by Wed", or "use by today" on its last day when that is today. */
  useBy: string | null
  /** Portions to reheat: the link's portions (component × eaters). */
  portions: number
}

export interface TodayRow {
  cell: BoardCell
  slotName: string
}

export interface TodayReminderView {
  key: string
  kind: "defrost" | "use_by"
  label: string
  text: string
}

export interface TodayRateView {
  plateId: number
  question: string
}

/** What `GET /api/today` alone gives: rendered before the plan loads. */
export interface TodayHeaderView {
  /** "Monday, 2 November". */
  dateLine: string
  /** The headline: exactly one verdict. */
  headline: string
  /** Null on a future day: its meals are not eaten yet. */
  rate: TodayRateView | null
  /** The next window's start while planning it is due, else null. */
  planNext: string | null
  /** Recipes still missing to reach the planning target; 0 when reached. */
  recipesMissing: number
  noPlan: boolean
  /** The window holds drafts but no confirmed plate on this day. */
  draftOnly: boolean
}

/** The parts that need the window's plan: colours, badges and slot names. */
export interface TodayPlanView {
  /** The window's board, for the week strip. */
  board: Board
  cook: TodayCookCard[]
  leftover: TodayLeftoverCard | null
  /** One row per meal slot, without the meal the leftover card shows. */
  rows: TodayRow[]
  reminders: TodayReminderView[]
}

/** The headline for a verdict (UX §5.2), e.g. "Cook Chili × 6". On a day
 *  other than today, `weekday` names it instead of saying "today". */
export function verdictText(
  verdict: TodayVerdict,
  f: Formatter,
  weekday?: string
): string {
  const key = weekday
    ? (`today.verdict_day.${verdict.kind}` as const)
    : (`today.verdict.${verdict.kind}` as const)
  switch (verdict.kind) {
    case "cook":
      return f.t(key, {
        weekday,
        name: verdict.food_name,
        portions: formatAmount(verdict.portions, f),
      })
    case "quick_cook":
      return f.t(key, { weekday, name: verdict.food_name })
    default:
      return f.t(key, { weekday })
  }
}

/** `iso` plus `n` calendar days. */
export function shiftDate(iso: string, n: number): string {
  const [y, m, d] = iso.split("-").map(Number)
  return new Date(Date.UTC(y, m - 1, d + n)).toISOString().slice(0, 10)
}

/**
 * The header of Today (UX §5.2) from `GET /api/today` alone: date line,
 * verdict, rating question, plan banner and setup state. `realToday` is the
 * client's date: the rating question only asks about days already lived.
 */
export function buildTodayHeader(
  today: Today,
  realToday: string,
  f: Formatter
): TodayHeaderView {
  return {
    dateLine: formatDate(today.date, f, {
      weekday: "long",
      day: "numeric",
      month: "long",
    }),
    headline: verdictText(
      today.verdict,
      f,
      today.date === realToday
        ? undefined
        : formatDate(today.date, f, { weekday: "long" })
    ),
    rate:
      today.rate && today.date <= realToday
        ? {
            plateId: today.rate.plate_id,
            question: f.t("today.rate.question", {
              weekday: formatDate(today.rate.date, f, { weekday: "long" }),
              name: today.rate.food_name,
            }),
          }
        : null,
    planNext: today.plan_banner.due ? today.plan_banner.next_start : null,
    recipesMissing: Math.max(
      0,
      today.setup.recipe_target - today.setup.recipe_count
    ),
    noPlan: today.verdict.kind === "no_plan",
    draftOnly: today.verdict.kind === "draft_only",
  }
}

/**
 * The plan-dependent parts of Today: `GET /api/today`'s cooking and
 * reminders joined with the window's board from `GET /api/plan` for batch
 * colours, badges, use-by dates and slot names. Pure: every domain fact
 * comes from the API.
 */
export function buildTodayPlan(
  today: Today,
  plan: Plan,
  realToday: string,
  f: Formatter
): TodayPlanView {
  const board = buildBoard(plan, f)
  const day = board.days.find((d) => d.date === today.date)
  const slotName = new Map(
    plan.slots.map((s) => [s.id, slotLabel(f.t, s.name_key)])
  )
  const nameOf = (id: number) => slotName.get(id) ?? ""
  const sessions = new Map(plan.sessions.map((s) => [s.id, s]))
  const colours = new Map(
    colourBatches(plan.sessions, plan.slots).map((b) => [
      b.session.id,
      b.colour,
    ])
  )

  const cook: TodayCookCard[] = today.cook.map(({ session, minutes }) => ({
    session,
    colour: colours.get(session.id) ?? 1,
    slotName: nameOf(session.cook_slot_id),
    coverage: coverageText(session.links, nameOf, f),
    // 0 is what an untouched recipe form saves: no estimate, not "0 min".
    minutes: minutes || null,
  }))

  // Today reads confirmed rows only (D26): a draft plate leaves its cell
  // as if unplanned.
  const cells = (day?.cells ?? []).map((c) =>
    c.plate?.state === "draft"
      ? {
          ...c,
          plate: null,
          badge: null,
          mainName: null,
          sideText: null,
          subline: null,
          sessionIds: [],
          cookOf: [],
        }
      : c
  )
  let leftover: TodayLeftoverCard | null = null
  if (cook.length === 0) {
    const cell = cells.find(
      (c) => c.badge?.variant === "leftover" || c.badge?.variant === "frozen"
    )
    if (cell?.badge) {
      const session = sessions.get(cell.badge.sessionId)
      const main = cell.plate?.components.find(
        (c) => c.cook_session_id === cell.badge?.sessionId
      )
      const link = session?.links.find((l) => l.component_id === main?.id)
      leftover = {
        cell,
        badge: cell.badge,
        slotName: nameOf(cell.slotId),
        useBy:
          cell.badge.variant === "leftover" && session
            ? session.use_by === realToday
              ? f.t("today.use_by_today")
              : f.t("today.use_by", {
                  day: formatDate(session.use_by, f, { weekday: "short" }),
                })
            : null,
        portions: link?.portions ?? cell.plate?.eaters ?? 1,
      }
    }
  }

  const rows = cells
    .filter((c) => c !== leftover?.cell)
    .map((cell) => ({ cell, slotName: nameOf(cell.slotId) }))

  const seen = new Set<string>()
  const reminders: TodayReminderView[] = []
  for (const r of today.reminders) {
    const key = `${r.kind}|${r.session_id}`
    if (seen.has(key)) continue
    seen.add(key)
    reminders.push(
      r.kind === "defrost"
        ? {
            key,
            kind: r.kind,
            label: f.t("today.reminder.freezer"),
            text: f.t("today.reminder.defrost", { name: r.food_name }),
          }
        : {
            key,
            kind: r.kind,
            label: f.t("today.reminder.last_day"),
            text: f.t("today.reminder.use_by", {
              slot: nameOf(r.slot_id),
              name: r.food_name,
            }),
          }
    )
  }

  return { board, cook, leftover, rows, reminders }
}
