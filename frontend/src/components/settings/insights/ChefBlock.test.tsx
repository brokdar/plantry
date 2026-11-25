import { screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { ChefInsights } from "@/lib/api/tips"
import i18n from "@/lib/i18n"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/tips", () => ({ getChefInsights: vi.fn() }))

import { getChefInsights } from "@/lib/api/tips"

import { ChefBlock } from "./ChefBlock"

const insights: ChefInsights = {
  on: true,
  from: "2026-09-30",
  to: "2026-11-25",
  tips: { shown: 8, applied: 2, dismissed: 3 },
  month: "2026-11",
  spend: [{ kind: "chef_attributes", usd: 0.42 }],
}

describe("ChefBlock", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
  })

  it("shows the tip counts, the apply rate and the spend by kind", async () => {
    vi.mocked(getChefInsights).mockResolvedValue(insights)
    renderWithRouter(<ChefBlock />)
    expect(
      await screen.findByRole("heading", { name: "Chef" })
    ).toBeInTheDocument()
    expect(screen.getByTestId("chef-tips-shown")).toHaveTextContent(
      "Tips shown8"
    )
    expect(screen.getByTestId("chef-tips-applied")).toHaveTextContent("2")
    expect(screen.getByTestId("chef-tips-dismissed")).toHaveTextContent("3")
    expect(screen.getByTestId("chef-tips-rate")).toHaveTextContent("25%")
    expect(screen.getByTestId("chef-spend-chef_attributes")).toHaveTextContent(
      "Spend: attribute labelling$0.42"
    )
  })

  it("shows a dash for the rate before any tip", async () => {
    vi.mocked(getChefInsights).mockResolvedValue({
      ...insights,
      tips: { shown: 0, applied: 0, dismissed: 0 },
      spend: [],
    })
    renderWithRouter(<ChefBlock />)
    expect(await screen.findByTestId("chef-tips-rate")).toHaveTextContent("–")
  })

  it("renders nothing while the Chef is off", async () => {
    vi.mocked(getChefInsights).mockResolvedValue({ ...insights, on: false })
    renderWithRouter(<ChefBlock />)
    await waitFor(() => expect(getChefInsights).toHaveBeenCalled())
    expect(screen.queryByTestId("insights-chef")).toBeNull()
  })
})
