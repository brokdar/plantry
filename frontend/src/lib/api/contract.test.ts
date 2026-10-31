// F1-AC1: every fetcher of the Phase 2 contract (§3) calls the documented
// method, path and query. Requests with a `today` parameter send todayISO()
// (D25).
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockRemovedBatch } from "@/test/fixtures"

vi.mock("./client", () => ({ apiFetch: vi.fn().mockResolvedValue({}) }))
vi.mock("@/lib/dates", () => ({ todayISO: () => "2026-10-30" }))

import { apiFetch } from "./client"
import { deleteFeedback, putFeedback } from "./feedback"
import { getFood, getScaled, listFoods, setShopSection } from "./foods"
import { clearPlan, getPlan, restorePlan } from "./plan"
import {
  addPlateComponent,
  createPlate,
  deletePlate,
  deletePlateComponent,
  updatePlate,
  updatePlateComponent,
} from "./plates"
import {
  addLeftovers,
  deleteSession,
  getLeftoverCells,
  getSession,
  setSessionCooked,
  updateSession,
} from "./sessions"
import { getShoppingList } from "./shopping"
import { getToday } from "./today"

interface Case {
  name: string
  call: () => Promise<unknown>
  method: string
  path: string
  body?: unknown
}

const snapshot = mockRemovedBatch.removed

const cases: Case[] = [
  // Plates (§3.1)
  {
    name: "createPlate",
    call: () =>
      createPlate({
        date: "2026-11-01",
        slot_id: 3,
        eaters: 2,
        components: [{ food_id: 42 }],
      }),
    method: "POST",
    path: "/plates",
    body: {
      date: "2026-11-01",
      slot_id: 3,
      eaters: 2,
      components: [{ food_id: 42 }],
    },
  },
  {
    name: "updatePlate",
    call: () => updatePlate(1, { eaters: 3.5, note: null }),
    method: "PUT",
    path: "/plates/1",
    body: { eaters: 3.5, note: null },
  },
  {
    name: "deletePlate",
    call: () => deletePlate(1),
    method: "DELETE",
    path: "/plates/1",
  },
  {
    name: "addPlateComponent",
    call: () =>
      addPlateComponent(2, { food_id: 42, portions: 1, cook_session_id: 7 }),
    method: "POST",
    path: "/plates/2/components",
    body: { food_id: 42, portions: 1, cook_session_id: 7 },
  },
  {
    name: "updatePlateComponent",
    call: () => updatePlateComponent(2, 10, { amount: 1, unit: "piece" }),
    method: "PUT",
    path: "/plates/2/components/10",
    body: { amount: 1, unit: "piece" },
  },
  {
    name: "deletePlateComponent",
    call: () => deletePlateComponent(2, 10),
    method: "DELETE",
    path: "/plates/2/components/10",
  },
  {
    name: "putFeedback",
    call: () => putFeedback(1, { status: "not_eaten", note: null }),
    method: "PUT",
    path: "/plates/1/feedback",
    body: { status: "not_eaten", note: null },
  },
  {
    name: "deleteFeedback",
    call: () => deleteFeedback(1),
    method: "DELETE",
    path: "/plates/1/feedback",
  },
  // Sessions (§3.1)
  {
    name: "getSession",
    call: () => getSession(7),
    method: "GET",
    path: "/cook-sessions/7",
  },
  {
    name: "updateSession",
    call: () =>
      updateSession(7, { portions: 8, frozen_portions: 2, note: null }),
    method: "PUT",
    path: "/cook-sessions/7",
    body: { portions: 8, frozen_portions: 2, note: null },
  },
  {
    name: "setSessionCooked",
    call: () => setSessionCooked(7, true),
    method: "POST",
    path: "/cook-sessions/7/cooked",
    body: { cooked: true },
  },
  {
    name: "getLeftoverCells",
    call: () => getLeftoverCells(7, 2),
    method: "GET",
    path: "/cook-sessions/7/leftover-cells?count=2",
  },
  {
    name: "addLeftovers",
    call: () =>
      addLeftovers(7, [
        { date: "2026-11-02", slot_id: 2 },
        { date: "2026-11-02", slot_id: 3 },
      ]),
    method: "POST",
    path: "/cook-sessions/7/leftovers",
    body: {
      cells: [
        { date: "2026-11-02", slot_id: 2 },
        { date: "2026-11-02", slot_id: 3 },
      ],
    },
  },
  {
    name: "deleteSession",
    call: () => deleteSession(7),
    method: "DELETE",
    path: "/cook-sessions/7",
  },
  // Plan (§3.1, §3.2)
  {
    name: "getPlan (anchored)",
    call: () => getPlan(),
    method: "GET",
    path: "/plan?today=2026-10-30",
  },
  {
    name: "getPlan (start)",
    call: () => getPlan("2026-10-31"),
    method: "GET",
    path: "/plan?start=2026-10-31&today=2026-10-30",
  },
  {
    name: "clearPlan",
    call: () => clearPlan("2026-10-31", "2026-11-06"),
    method: "DELETE",
    path: "/plan?from=2026-10-31&to=2026-11-06",
  },
  {
    name: "restorePlan",
    call: () => restorePlan(snapshot),
    method: "POST",
    path: "/plan/restore",
    body: snapshot,
  },
  {
    name: "getToday",
    call: () => getToday("2026-11-02"),
    method: "GET",
    path: "/today?date=2026-11-02&today=2026-10-30",
  },
  // Shopping and foods (§3.3)
  {
    name: "getShoppingList (default range)",
    call: () => getShoppingList(),
    method: "GET",
    path: "/shopping-list?today=2026-10-30",
  },
  {
    name: "getShoppingList (range)",
    call: () => getShoppingList({ from: "2026-11-01", to: "2026-11-06" }),
    method: "GET",
    path: "/shopping-list?from=2026-11-01&to=2026-11-06&today=2026-10-30",
  },
  {
    name: "listFoods (new params)",
    call: () =>
      listFoods({
        kind: "composed",
        max_minutes: 30,
        not_eaten_weeks: 4,
        created_within_days: 30,
        sort: "last_eaten",
      }),
    method: "GET",
    path: "/foods?kind=composed&max_minutes=30&not_eaten_weeks=4&created_within_days=30&sort=last_eaten&today=2026-10-30",
  },
  {
    name: "getFood",
    call: () => getFood(42),
    method: "GET",
    path: "/foods/42?today=2026-10-30",
  },
  {
    name: "getScaled",
    call: () => getScaled(42, 6),
    method: "GET",
    path: "/foods/42/scaled?portions=6",
  },
  {
    name: "setShopSection",
    call: () => setShopSection(50, "frozen"),
    method: "PUT",
    path: "/foods/50/shop-section",
    body: { section: "frozen" },
  },
  {
    name: "setShopSection (revert)",
    call: () => setShopSection(50, null),
    method: "PUT",
    path: "/foods/50/shop-section",
    body: { section: null },
  },
]

describe("API contract (§3)", () => {
  beforeEach(() => vi.mocked(apiFetch).mockClear())

  it.each(cases)("$name → $method $path", async (c) => {
    await c.call()
    expect(apiFetch).toHaveBeenCalledTimes(1)
    const [path, init] = vi.mocked(apiFetch).mock.calls[0]
    expect(path).toBe(c.path)
    expect(init?.method ?? "GET").toBe(c.method)
    if (c.body === undefined) {
      expect(init?.body).toBeUndefined()
    } else {
      expect(JSON.parse(init!.body as string)).toEqual(c.body)
    }
  })
})
