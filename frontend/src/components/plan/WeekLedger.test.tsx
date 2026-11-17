import { cleanup, render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { SoftIssue, WeekLedger as Week } from "@/lib/api/plan"
import i18n from "@/lib/i18n"
import { buildBoard } from "@/lib/plan/board"
import { dayMeterTargets } from "@/lib/plan/ledger"
import { mockPlan } from "@/test/fixtures"

import { DayList } from "./DayList"
import { WeekLedger } from "./WeekLedger"

const week: Week = {
  days: 5,
  avg_kcal: 2150,
  target_kcal: 2200,
  pct: 97.7,
  avg_protein_g: 80,
  target_protein_g: 75,
  protein_pct: 106.7,
  carbs_pct: 50,
  fat_pct: 30,
  fiber_g: 28,
  meat_g: 120,
  fish_meals: 1,
  legume_meals: 1,
}

function soft(rule: SoftIssue["rule"], code: string, extra = {}): SoftIssue {
  return { rule, code, severity: "soft", params: {}, ...extra }
}

function ledger(w: Week = week, s: SoftIssue[] = []) {
  return render(<WeekLedger week={w} soft={s} />)
}

const status = (key: string) =>
  screen.getByTestId(`ledger-${key}`).getAttribute("data-status")

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

describe("WeekLedger", () => {
  it("shows every value quietly while the week is in range", () => {
    ledger()
    expect(screen.getByRole("region", { name: "Week nutrition" })).toBeVisible()
    const kcal = within(screen.getByTestId("ledger-kcal"))
    expect(kcal.getByText("98 %")).toBeVisible()
    expect(kcal.getByText("2,150 of 2,200 kcal a day")).toBeVisible()
    expect(kcal.getByText("in range")).toBeVisible()
    for (const key of ["kcal", "protein", "carbs", "fat"]) {
      expect(status(key)).toBe("ok")
    }
    expect(status("fiber")).toBe("info")
    expect(screen.getByText("28 g")).toBeVisible()
    expect(screen.queryByTestId("ledger-dge")).not.toBeInTheDocument()
    expect(screen.queryByTestId("ledger-day-hints")).not.toBeInTheDocument()
  })

  it("highlights a week below its band, in words as well as colour", () => {
    ledger({ ...week, pct: 88 }, [soft("S4", "week_kcal_below_band")])
    expect(status("kcal")).toBe("low")
    expect(
      within(screen.getByTestId("ledger-kcal")).getByText("below")
    ).toBeVisible()
    expect(status("protein")).toBe("ok")
  })

  it("highlights a week above its band", () => {
    ledger({ ...week, pct: 112 }, [soft("S4", "week_kcal_above_band")])
    expect(status("kcal")).toBe("high")
    expect(
      within(screen.getByTestId("ledger-kcal")).getByText("above")
    ).toBeVisible()
  })

  it("marks protein, carbs and fat that are out of range, on the right side", () => {
    ledger({ ...week, protein_pct: 80, carbs_pct: 70, fat_pct: 15 }, [
      soft("S3", "week_protein_below_target"),
      soft("S5", "week_carbs_out_of_range", {
        params: { range_low_pct: 45, range_high_pct: 65 },
      }),
      soft("S5", "week_fat_out_of_range", {
        params: { range_low_pct: 20, range_high_pct: 35 },
      }),
    ])
    expect(status("protein")).toBe("low")
    expect(status("carbs")).toBe("high")
    expect(status("fat")).toBe("low")
  })

  it("without a weight shows the protein as an average and no verdict", async () => {
    ledger({ ...week, target_protein_g: undefined, protein_pct: undefined })
    expect(status("protein")).toBe("info")
    expect(screen.getByText("Avg. 80 g a day")).toBeVisible()
    cleanup()
    await i18n.changeLanguage("de")
    ledger({ ...week, target_protein_g: undefined, protein_pct: undefined })
    expect(screen.getByText("Ø 80 g am Tag")).toBeVisible()
  })

  it("leaves fibre out when the server has none, and counts light days", () => {
    ledger({ ...week, fiber_g: undefined, normal_target_kcal: 2300 })
    expect(screen.queryByTestId("ledger-fiber")).not.toBeInTheDocument()
    expect(
      screen.getByText("2,150 of 2,200 kcal a day, light days counted")
    ).toBeVisible()
  })

  it("shows the DGE line only for the rules that are off", () => {
    ledger(week, [
      soft("S7", "week_meat_above_dge", {
        params: { meat_g: 410, max_g: 300 },
      }),
      soft("S7", "week_legumes_below_dge"),
    ])
    const dge = within(screen.getByTestId("ledger-dge"))
    expect(
      dge.getByText("Meat: 410 g this week, over the 300 g DGE limit.")
    ).toBeVisible()
    expect(dge.getByText(/No legume meal/)).toBeVisible()
    expect(dge.queryByText(/No fish meal/)).not.toBeInTheDocument()
  })

  it("keeps hints on single days in a closed details element", () => {
    ledger(week, [
      soft("S2", "kcal_below_band", {
        date: "2026-11-03",
        params: { kcal: 1100, band_low_kcal: 1980, band_high_kcal: 2420 },
      }),
    ])
    const details = screen.getByTestId("ledger-day-hints")
    expect(details).not.toHaveAttribute("open")
    expect(within(details).getByText("1 hint on single days")).toBeVisible()
    expect(details.textContent).toContain("1,100 kcal, below the 1,980–2,420")
  })

  it("speaks German", async () => {
    await i18n.changeLanguage("de")
    ledger({ ...week, pct: 88 }, [soft("S4", "week_kcal_below_band")])
    expect(screen.getByRole("region", { name: "Wochenbilanz" })).toBeVisible()
    expect(screen.getByText("darunter")).toBeVisible()
  })
})

describe("light marks", () => {
  const light = mockPlan.days.map((d) =>
    d.date === "2026-11-02"
      ? { ...d, light: "day" as const, target_kcal: 1870 }
      : d.date === "2026-11-03"
        ? { ...d, light: "meal" as const, light_slot: "dinner" }
        : d
  )

  it("marks a light day and a light meal on the phone list, with words", () => {
    const board = buildBoard({ ...mockPlan, days: light }, i18n)
    expect(board.days.map((d) => d.light)).toEqual([
      null,
      null,
      "Light day",
      "Light: Dinner",
      null,
      null,
      null,
    ])
    render(
      <DayList
        board={board}
        targets={mockPlan.targets}
        label="Plan"
        onOpenCell={() => {}}
        onMarkNoPlate={() => {}}
      />
    )
    expect(screen.getAllByTestId("light-mark")).toHaveLength(2)
    expect(screen.getByText("Light day")).toBeVisible()
    expect(screen.getByText("Light: Dinner")).toBeVisible()
  })

  it("labels a hearty request in en and de", async () => {
    const hearty = light.map((d) =>
      d.light ? { ...d, light_level: "hearty" as const } : d
    )
    const board = buildBoard({ ...mockPlan, days: hearty }, i18n)
    expect(board.days.filter((d) => d.light).map((d) => d.light)).toEqual([
      "Hearty day",
      "Hearty: Dinner",
    ])
    expect(board.days.filter((d) => d.light).every((d) => d.hearty)).toBe(true)
    await i18n.changeLanguage("de")
    const de = buildBoard({ ...mockPlan, days: hearty }, i18n)
    expect(de.days.filter((d) => d.light).map((d) => d.light)).toEqual([
      "Deftiger Tag",
      "Deftig: Abendessen",
    ])
    await i18n.changeLanguage("en")
  })

  it("measures a light day against its adjusted target only", () => {
    const [plain, day, meal] = [light[0], light[2], light[3]]
    const t = mockPlan.targets
    expect(dayMeterTargets(t, plain)).toBe(t)
    expect(dayMeterTargets(t, meal)).toBe(t)
    expect(dayMeterTargets(t, day)?.kcal).toBe(1870)
    expect(dayMeterTargets(null, day)).toBeNull()
  })
})

describe("WeekLedger on a phone", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn().mockImplementation((query: string) => ({
        matches: query.includes("max-width: 767px"),
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }))
    )
  })
  afterEach(() => vi.unstubAllGlobals())

  it("is one calm line while everything is in range, open on tap", async () => {
    ledger()
    const toggle = screen.getByRole("button", { name: "Week on track" })
    expect(toggle).toHaveAttribute("aria-expanded", "false")
    expect(screen.queryByTestId("ledger-kcal")).not.toBeInTheDocument()
    await userEvent.click(toggle)
    expect(toggle).toHaveAttribute("aria-expanded", "true")
    expect(screen.getByTestId("ledger-kcal")).toBeVisible()
  })

  it("names only what is off, with arrows", () => {
    ledger({ ...week, pct: 27, fat_pct: 50 }, [
      soft("S4", "week_kcal_below_band"),
      soft("S5", "week_fat_out_of_range", {
        params: { range_low_pct: 20, range_high_pct: 35 },
      }),
    ])
    const toggle = screen.getByTestId("ledger-toggle")
    expect(toggle).toHaveTextContent("Energy 27 %")
    expect(toggle).toHaveTextContent("Fat")
    expect(toggle).toHaveTextContent("below")
    expect(toggle).toHaveTextContent("above")
    expect(toggle).not.toHaveTextContent("Protein")
    expect(toggle).not.toHaveTextContent("Week on track")
    expect(toggle).toHaveAttribute("aria-expanded", "false")
  })
})
