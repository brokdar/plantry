import type { AvailabilityDefault } from "@/lib/api/availability"
import type { ComposedFood, LeafFood, ScaledRecipe } from "@/lib/api/foods"
import type { LookupCandidate, LookupResponse } from "@/lib/api/lookup"
import type { Plan, PlanCell } from "@/lib/api/plan"
import type { Plate, PlateComponent, RemovedResponse } from "@/lib/api/plates"
import type { Session } from "@/lib/api/sessions"
import type { ShoppingList } from "@/lib/api/shopping"
import type { Today, TodayVerdictKind } from "@/lib/api/today"
import type { Profile } from "@/lib/api/profile"
import type { SettingItem, SettingsList, SystemInfo } from "@/lib/api/settings"
import type { TimeSlot } from "@/lib/api/slots"

export const mockChickenBreast: LeafFood = {
  id: 1,
  kind: "leaf",
  name: "Chicken breast",
  source: "manual",
  barcode: null,
  off_id: null,
  fdc_id: null,
  image_path: null,
  favorite: false,
  shop_section: "meat_fish",
  shop_section_source: "auto",
  used_in_count: 1,
  kcal_100g: 165,
  protein_100g: 31,
  fat_100g: 3.6,
  carbs_100g: 0,
  fiber_100g: 0,
  sodium_100g: 0,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
}

export const mockBrownRice: LeafFood = {
  id: 2,
  kind: "leaf",
  name: "Brown rice",
  source: "manual",
  barcode: null,
  off_id: null,
  fdc_id: null,
  image_path: null,
  favorite: false,
  shop_section: "dry_goods",
  shop_section_source: "auto",
  used_in_count: 0,
  kcal_100g: 112,
  protein_100g: 2.3,
  fat_100g: 0.8,
  carbs_100g: 24,
  fiber_100g: 1.8,
  sodium_100g: 1,
  created_at: "2024-01-02T00:00:00Z",
  updated_at: "2024-01-02T00:00:00Z",
}

export const mockChickenCurry: ComposedFood = {
  id: 3,
  kind: "composed",
  name: "Chicken Curry",
  role: "main",
  variant_group_id: null,
  reference_portions: 2,
  prep_minutes: 10,
  cook_minutes: 30,
  image_path: null,
  notes: null,
  favorite: false,
  last_eaten: null,
  times_cooked: 0,
  per_portion: { kcal: 0, protein: 0, complete: true },
  children: [
    {
      id: 1,
      parent_id: 3,
      child_id: 1,
      child_name: "Chicken breast",
      child_kind: "leaf",
      amount: 300,
      unit: "g",
      grams: 300,
      sort_order: 0,
    },
  ],
  instructions: [
    { id: 1, food_id: 3, step_number: 1, text: "Cook chicken" },
    { id: 2, food_id: 3, step_number: 2, text: "Add curry paste" },
  ],
  tags: ["spicy", "thai"],
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
}

export const mockTofuBowl: ComposedFood = {
  id: 4,
  kind: "composed",
  name: "Tofu Bowl",
  role: "standalone",
  variant_group_id: null,
  reference_portions: 1,
  prep_minutes: 5,
  cook_minutes: 15,
  image_path: null,
  notes: null,
  favorite: false,
  last_eaten: null,
  times_cooked: 0,
  per_portion: { kcal: 0, protein: 0, complete: true },
  children: [],
  instructions: [],
  tags: ["vegan"],
  created_at: "2024-01-02T00:00:00Z",
  updated_at: "2024-01-02T00:00:00Z",
}

export const mockChickenCurryWithVariantGroup: ComposedFood = {
  ...mockChickenCurry,
  id: 10,
  variant_group_id: 1,
}

export const mockTofuCurryVariant: ComposedFood = {
  id: 11,
  kind: "composed",
  name: "Tofu Curry",
  role: "main",
  variant_group_id: 1,
  reference_portions: 2,
  prep_minutes: 10,
  cook_minutes: 25,
  image_path: null,
  notes: null,
  favorite: false,
  last_eaten: null,
  times_cooked: 0,
  per_portion: { kcal: 0, protein: 0, complete: true },
  children: [],
  instructions: [],
  tags: ["vegan"],
  created_at: "2024-01-03T00:00:00Z",
  updated_at: "2024-01-03T00:00:00Z",
}

export const mockLookupCandidate: LookupCandidate = {
  name: "Chicken Breast, Raw",
  source: "fdc",
  barcode: null,
  fdc_id: 171077,
  image_url: null,
  existing_id: null,
  kcal_100g: 120,
  protein_100g: 22.5,
  fat_100g: 2.6,
  carbs_100g: 0,
  fiber_100g: 0,
  sodium_100g: 0.074,
  portions: [],
}

export const mockLookupResponse: LookupResponse = {
  results: [mockLookupCandidate],
  recommended_index: 0,
}

/** A composed plate component with sensible defaults. */
export function mockPlateComponent(
  overrides: Partial<PlateComponent> = {}
): PlateComponent {
  return {
    id: 1,
    plate_id: 1,
    food_id: 3,
    food_name: "Chicken Curry",
    kind: "composed",
    role: "main",
    portions: 1,
    cook_session_id: 1,
    sort_order: 0,
    ...overrides,
  }
}

/** An empty plate with sensible defaults. */
export function mockPlate(overrides: Partial<Plate> = {}): Plate {
  return {
    id: 1,
    date: "2026-04-26",
    slot_id: 1,
    eaters: 1,
    note: null,
    components: [],
    feedback: null,
    created_at: "2026-04-26T10:00:00Z",
    updated_at: "2026-04-26T10:00:00Z",
    ...overrides,
  }
}

// Availability: three active slots (1 breakfast, 2 lunch, 3 dinner).
// Weekday 0 = Monday … 6 = Sunday.
export const mockAvailabilityDefaults: AvailabilityDefault[] = [
  0, 1, 2, 3, 4, 5, 6,
].flatMap((weekday) =>
  [1, 2, 3].map((slot_id) => {
    if (slot_id === 1)
      return { weekday, slot_id, mode: "routine" as const, preset_id: 7 }
    if (slot_id === 2 && weekday < 5)
      return { weekday, slot_id, mode: "none" as const, preset_id: null }
    return { weekday, slot_id, mode: "plan" as const, preset_id: null }
  })
)

/** Props-shaped data for AvailabilityGrid tests. */
export const mockAvailabilityColumns = [
  { key: "0", label: "Monday" },
  { key: "1", label: "Tuesday" },
  { key: "2", label: "Wednesday" },
  { key: "3", label: "Thursday" },
  { key: "4", label: "Friday" },
  { key: "5", label: "Saturday" },
  { key: "6", label: "Sunday" },
]

export const mockAvailabilitySlots = [
  { id: 1, label: "Breakfast" },
  { id: 2, label: "Lunch" },
  { id: 3, label: "Dinner" },
]

export const mockAvailabilityPresets = [
  { id: 7, name: "Overnight oats" },
  { id: 8, name: "Bread and soup" },
]

// Profile with the Phase 1 defaults and no nutrition goal.
export const mockProfile: Profile = {
  kcal_target: null,
  protein_pct: null,
  fat_pct: null,
  carbs_pct: null,
  dietary_restrictions: [],
  preferences: {},
  system_prompt: null,
  locale: "en",
  diet_type: "omnivore",
  meat_days_per_week_max: null,
  household_portions: 1,
  updated_at: "2026-01-01T00:00:00Z",
}

export function mockSettingItem(
  key: string,
  value: string,
  source: SettingItem["source"] = "default",
  extra: Partial<SettingItem> = {}
): SettingItem {
  return { key, value, source, is_secret: false, env_also_set: false, ...extra }
}

/** Every editable settings key at its registry default (plan D4). */
export const mockSettingDefaults: Record<string, string> = {
  "ai.provider": "",
  "ai.model": "",
  "ai.api_key": "",
  "ai.rate_limit_per_min": "10",
  "ai.fake_script": "",
  "fdc.api_key": "",
  "plan.anchor": "next_shopping_day",
  "plan.shopping_day": "0",
  "setup.completed": "true",
  "plan.max_cook_occasions_per_day": "1",
  "plan.batch_cooking": "true",
  "plan.default_keep_days": "3",
  "plan.freezing_enabled": "false",
  "plan.freezer_keep_days": "90",
  "plan.variety_weeks": "3",
  "plan.explore_per_week": "0",
  "discovery.daily_enabled": "true",
  "ai.run_retention_days": "180",
}

const SECRET_KEYS = new Set(["ai.api_key", "fdc.api_key"])

/** The settings list with every key at its default; `overrides` replace
 *  single items by key. */
export function mockSettingsList(
  overrides: Partial<SettingItem>[] = []
): SettingsList {
  const items = Object.entries(mockSettingDefaults).map(([key, value]) => {
    const base = mockSettingItem(key, value, "default", {
      is_secret: SECRET_KEYS.has(key),
    })
    const override = overrides.find((o) => o.key === key)
    return override ? { ...base, ...override } : base
  })
  return { items, cipher_available: true }
}

export const mockSystemInfo: SystemInfo = {
  port: 8080,
  db_path: "/data/plantry.db",
  log_level: "info",
  image_path: "/data/images",
  dev_mode: false,
  version: "1.2.3",
  build_commit: "abc1234",
  cipher_available: true,
}

export const mockTimeSlots: TimeSlot[] = [
  {
    id: 1,
    name_key: "slot.breakfast",
    icon: "Coffee",
    sort_order: 0,
    active: true,
  },
  { id: 2, name_key: "slot.lunch", icon: "Sun", sort_order: 1, active: true },
  { id: 3, name_key: "slot.dinner", icon: "Moon", sort_order: 2, active: true },
]

// ── Phase 2 contract fixtures (§3) ───────────────────────────────────
// AC-1 "Sunday batch": household of 2, Chili (reference 4 portions: 400 g
// kidney beans, 2 pcs onion) cooked Sunday dinner as a batch of 3 meals,
// eaten again Monday lunch and Monday dinner. Window Sat 2026-10-31 to Fri
// 2026-11-06 (shopping day 5 = Saturday), slots 1–3 as mockTimeSlots.

export const mockChili: ComposedFood = {
  id: 42,
  kind: "composed",
  name: "Chili",
  role: "main",
  variant_group_id: null,
  reference_portions: 4,
  prep_minutes: 15,
  cook_minutes: 45,
  image_path: null,
  notes: null,
  keep_days: 3,
  freezable: true,
  favorite: false,
  children: [
    {
      id: 21,
      parent_id: 42,
      child_id: 50,
      child_name: "Kidney beans",
      child_kind: "leaf",
      amount: 400,
      unit: "g",
      grams: 400,
      sort_order: 0,
    },
    {
      id: 22,
      parent_id: 42,
      child_id: 51,
      child_name: "Onion",
      child_kind: "leaf",
      amount: 2,
      unit: "piece",
      grams: 240,
      grams_source: "portion",
      sort_order: 1,
    },
  ],
  instructions: [],
  tags: [],
  last_eaten: null,
  times_cooked: 0,
  per_portion: { kcal: 520, protein: 28, complete: true },
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
}

/** The AC-1 batch: Sun dinner (fresh), Mon lunch and Mon dinner. */
export const mockSession: Session = {
  id: 7,
  food_id: 42,
  food_name: "Chili",
  cook_date: "2026-11-01",
  cook_slot_id: 3,
  portions: 6,
  frozen_portions: 0,
  cooked_at: null,
  note: null,
  keep_days: 3,
  use_by: "2026-11-04",
  consumed: 6,
  frozen_consumed: 0,
  surplus: 0,
  links: [
    {
      plate_id: 1,
      component_id: 9,
      date: "2026-11-01",
      slot_id: 3,
      portions: 2,
      kind: "fresh",
      position: 1,
    },
    {
      plate_id: 2,
      component_id: 10,
      date: "2026-11-02",
      slot_id: 2,
      portions: 2,
      kind: "leftover",
      position: 2,
    },
    {
      plate_id: 3,
      component_id: 11,
      date: "2026-11-02",
      slot_id: 3,
      portions: 2,
      kind: "leftover",
      position: 3,
    },
  ],
  meals: 3,
  warnings: [],
}

/** A batch that needs attention: cooked for 4 but eats 6, and a leftover
 *  sits before its cook cell after a move. */
export const mockSessionWithWarnings: Session = {
  ...mockSession,
  id: 8,
  portions: 4,
  surplus: 0,
  warnings: [
    { code: "deficit", plate_id: null },
    { code: "leftover_before_cook", plate_id: 2 },
  ],
}

function chiliPlate(id: number, date: string, slotId: number): Plate {
  return mockPlate({
    id,
    date,
    slot_id: slotId,
    eaters: 2,
    components: [
      mockPlateComponent({
        id: id + 8,
        plate_id: id,
        food_id: 42,
        food_name: "Chili",
        cook_session_id: 7,
      }),
    ],
    created_at: "2026-10-30T10:00:00Z",
    updated_at: "2026-10-30T10:00:00Z",
  })
}

export const mockAc1Plates: Plate[] = [
  chiliPlate(1, "2026-11-01", 3),
  chiliPlate(2, "2026-11-02", 2),
  chiliPlate(3, "2026-11-02", 3),
]

const AC1_DATES = [
  "2026-10-31",
  "2026-11-01",
  "2026-11-02",
  "2026-11-03",
  "2026-11-04",
  "2026-11-05",
  "2026-11-06",
]

const ac1Cells: PlanCell[] = AC1_DATES.flatMap((date) =>
  [1, 2, 3].map((slot_id) => ({
    date,
    slot_id,
    mode: "plan" as const,
    source: "default" as const,
    preset_id: null,
    preset_name: null,
    note: null,
    plate:
      mockAc1Plates.find((p) => p.date === date && p.slot_id === slot_id) ??
      null,
  }))
)

/** GET /api/plan for the AC-1 window: 21 cells, one session of 3 meals, and
 *  Monday's per-eater kcal = 2 × Chili's per-portion kcal. */
export const mockPlan: Plan = {
  window: {
    start: "2026-10-31",
    end: "2026-11-06",
    today: "2026-10-31",
    is_current: true,
    prev_start: "2026-10-24",
    next_start: "2026-11-07",
    shopping_day: 5,
  },
  slots: mockTimeSlots.map(({ id, name_key, icon, sort_order }) => ({
    id,
    name_key,
    icon,
    sort_order,
  })),
  cells: ac1Cells,
  sessions: [mockSession],
  days: AC1_DATES.map((date) => {
    const portions = mockAc1Plates.filter((p) => p.date === date).length
    return {
      date,
      macros: {
        kcal: 520 * portions,
        protein: 28 * portions,
        fat: 18 * portions,
        carbs: 60 * portions,
      },
      complete: true,
      missing: [],
    }
  }),
  targets: { kcal: 2200, protein_g: 140, fat_g: 70, carbs_g: 250 },
  stats: { plates: 3, cook_days: 1, warnings: 0 },
}

const mockTodayBase: Omit<Today, "verdict"> = {
  date: "2026-11-02",
  window_start: "2026-10-31",
  cook: [],
  meals: [
    {
      slot_id: 1,
      mode: "plan",
      preset_name: null,
      note: null,
      plate: null,
      link: null,
    },
    {
      slot_id: 2,
      mode: "plan",
      preset_name: null,
      note: null,
      plate: mockAc1Plates[1],
      link: mockSession.links[1],
    },
    {
      slot_id: 3,
      mode: "plan",
      preset_name: null,
      note: null,
      plate: mockAc1Plates[2],
      link: mockSession.links[2],
    },
  ],
  reminders: [],
  rate: null,
  plan_banner: { due: false, next_start: "2026-11-07" },
  setup: { recipe_count: 12, recipe_target: 10 },
}

/** GET /api/today, one response per verdict. `cook` is AC-1's Sunday,
 *  `nothing_to_cook` its Monday. */
export const mockTodayByVerdict: Record<TodayVerdictKind, Today> = {
  cook: {
    ...mockTodayBase,
    date: "2026-11-01",
    verdict: {
      kind: "cook",
      session_id: 7,
      food_name: "Chili",
      portions: 6,
      slot_id: 3,
    },
    cook: [{ session: mockSession, minutes: 60 }],
    meals: [
      { ...mockTodayBase.meals[0] },
      { ...mockTodayBase.meals[1], plate: null, link: null },
      {
        ...mockTodayBase.meals[2],
        plate: mockAc1Plates[0],
        link: mockSession.links[0],
      },
    ],
  },
  quick_cook: {
    ...mockTodayBase,
    date: "2026-11-03",
    verdict: {
      kind: "quick_cook",
      session_id: 9,
      food_name: "Omelette",
      portions: 2,
      slot_id: 1,
    },
    cook: [
      {
        session: {
          ...mockSession,
          id: 9,
          food_id: 60,
          food_name: "Omelette",
          cook_date: "2026-11-03",
          cook_slot_id: 1,
          portions: 2,
          consumed: 2,
          use_by: "2026-11-06",
          links: [
            {
              plate_id: 4,
              component_id: 12,
              date: "2026-11-03",
              slot_id: 1,
              portions: 2,
              kind: "fresh",
              position: 1,
            },
          ],
          meals: 1,
        },
        minutes: 15,
      },
    ],
  },
  nothing_to_cook: {
    ...mockTodayBase,
    verdict: { kind: "nothing_to_cook" },
    rate: {
      plate_id: 1,
      date: "2026-11-01",
      slot_id: 3,
      food_name: "Chili",
    },
  },
  no_plates: {
    ...mockTodayBase,
    date: "2026-11-05",
    verdict: { kind: "no_plates" },
    meals: mockTodayBase.meals.map((m) => ({ ...m, plate: null, link: null })),
    plan_banner: { due: false, next_start: "2026-11-07" },
  },
  no_plan: {
    ...mockTodayBase,
    date: "2026-11-09",
    window_start: "2026-11-07",
    verdict: { kind: "no_plan" },
    meals: mockTodayBase.meals.map((m) => ({ ...m, plate: null, link: null })),
    plan_banner: { due: true, next_start: "2026-11-14" },
  },
}

/** GET /api/shopping-list for the AC-1 window: beans 600 g, onions 3 pcs. */
export const mockShoppingList: ShoppingList = {
  range: {
    from: "2026-10-31",
    to: "2026-11-06",
    window_start: "2026-10-31",
    window_end: "2026-11-06",
  },
  sections: [
    {
      section: "produce",
      items: [
        {
          food_id: 51,
          name: "Onion",
          total_grams: 360,
          display: { amount: 3, unit: "piece" },
          buy: { amount: 3, unit: "piece" },
          used_in: [
            {
              session_id: 7,
              food_id: 42,
              food_name: "Chili",
              date: "2026-11-01",
              slot_id: 3,
            },
          ],
        },
      ],
    },
    {
      section: "cans_jars",
      items: [
        {
          food_id: 50,
          name: "Kidney beans",
          total_grams: 600,
          display: { amount: 600, unit: "g" },
          buy: null,
          used_in: [
            {
              session_id: 7,
              food_id: 42,
              food_name: "Chili",
              date: "2026-11-01",
              slot_id: 3,
            },
          ],
        },
      ],
    },
  ],
  count: 2,
}

/** GET /api/foods/42/scaled?portions=6 (factor 6/4). */
export const mockScaled: ScaledRecipe = {
  food_id: 42,
  portions: 6,
  reference_portions: 4,
  factor: 1.5,
  ingredients: [
    {
      child_id: 50,
      name: "Kidney beans",
      kind: "leaf",
      amount: 600,
      unit: "g",
      grams: 600,
      display: { amount: 600, unit: "g" },
      buy: null,
    },
    {
      child_id: 51,
      name: "Onion",
      kind: "leaf",
      amount: 3,
      unit: "piece",
      grams: 360,
      display: { amount: 3, unit: "piece" },
      buy: { amount: 3, unit: "piece" },
    },
  ],
}

/** DELETE /api/cook-sessions/7: the AC-1 batch and its three plates. */
export const mockRemovedBatch: RemovedResponse = {
  removed: {
    plates: mockAc1Plates.map((p) => ({
      date: p.date,
      slot_id: p.slot_id,
      eaters: p.eaters,
      note: null,
      components: [
        { food_id: 42, portions: 1, sort_order: 0, session: { ref: "s1" } },
      ],
    })),
    sessions: [
      {
        ref: "s1",
        food_id: 42,
        cook_date: "2026-11-01",
        cook_slot_id: 3,
        portions: 6,
        frozen_portions: 0,
        cooked_at: null,
        note: null,
      },
    ],
  },
}
