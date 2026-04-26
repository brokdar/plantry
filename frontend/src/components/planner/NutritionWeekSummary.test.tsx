import { screen } from "@testing-library/react"
import { describe, it, expect, vi, beforeEach } from "vitest"

import { renderWithRouter } from "@/test/render"

// Mock the nutrition API module before importing the component
vi.mock("@/lib/api/nutrition", () => ({
  getNutritionRange: vi.fn().mockResolvedValue({ days: [] }),
}))

// Mock the profile query
vi.mock("@/lib/queries/profile", () => ({
  useProfile: vi.fn().mockReturnValue({ data: undefined }),
}))

import { getNutritionRange } from "@/lib/api/nutrition"
import { NutritionWeekSummary } from "./NutritionWeekSummary"

describe("NutritionWeekSummary", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("calls range endpoint with the provided from/to dates", async () => {
    renderWithRouter(<NutritionWeekSummary from="2026-04-26" to="2026-05-02" />)

    await screen.findByText(/loading|empty/i)

    expect(getNutritionRange).toHaveBeenCalledWith("2026-04-26", "2026-05-02")
  })

  it("does not call the week-based nutrition endpoint", async () => {
    renderWithRouter(<NutritionWeekSummary from="2026-04-26" to="2026-05-02" />)

    await screen.findByText(/loading|empty/i)

    // getNutritionRange is the only nutrition fetch; the old week-scoped
    // endpoint (/api/weeks/:id/nutrition) has no import path in this module
    expect(getNutritionRange).toHaveBeenCalledTimes(1)
    expect(getNutritionRange).not.toHaveBeenCalledWith(
      expect.stringContaining("weeks")
    )
  })
})
