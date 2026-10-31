import type { AvailabilityMode } from "./availability"
import { apiFetch } from "./client"
import type { Plate } from "./plates"
import type { Session, SessionLink } from "./sessions"

/** The day's headline (§3.2 verdict rules, in order). */
export type TodayVerdict =
  | {
      kind: "cook" | "quick_cook"
      session_id: number
      food_name: string
      portions: number
      slot_id: number
    }
  | { kind: "nothing_to_cook" | "draft_only" | "no_plates" | "no_plan" }

export type TodayVerdictKind = TodayVerdict["kind"]

export interface TodayCook {
  session: Session
  /** Prep + cook minutes of the recipe, null when unknown. */
  minutes: number | null
}

export interface TodayMeal {
  slot_id: number
  mode: AvailabilityMode
  preset_name: string | null
  note: string | null
  plate: Plate | null
  link: SessionLink | null
}

export interface TodayReminder {
  kind: "defrost" | "use_by"
  session_id: number
  food_name: string
  date: string
  slot_id: number
}

export interface TodayRate {
  plate_id: number
  date: string
  slot_id: number
  food_name: string
}

export interface Today {
  date: string
  window_start: string
  verdict: TodayVerdict
  cook: TodayCook[]
  meals: TodayMeal[]
  reminders: TodayReminder[]
  rate: TodayRate | null
  plan_banner: { due: boolean; next_start: string }
  setup: { recipe_count: number; recipe_target: number }
}

/** GET /api/today?date&today. */
export function getToday(date: string, today: string): Promise<Today> {
  const qs = new URLSearchParams({ date, today })
  return apiFetch(`/today?${qs}`)
}
