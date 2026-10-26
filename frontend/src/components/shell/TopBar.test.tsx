import { describe, expect, test, vi } from "vitest"
import { screen, within } from "@testing-library/react"

import { renderWithRouter } from "@/test/render"
import { TopBar } from "./TopBar"

vi.mock("@/lib/api/setup", () => ({
  getSetupStatus: vi.fn().mockResolvedValue({
    completed: true,
    recipe_count: 10,
    recipe_target: 10,
  }),
  completeSetup: vi.fn(),
}))

describe("TopBar", () => {
  test("shows a settings gear only below 768 px", async () => {
    renderWithRouter(<TopBar />)
    const gear = await screen.findByRole("link", { name: /settings/i })
    expect(gear).toHaveAttribute("href", "/settings")
    // md (768 px) and up hides it; the rail carries Settings there.
    expect(gear).toHaveClass("md:hidden")
  })

  test("the whole bar is hidden from md up", async () => {
    renderWithRouter(<TopBar />)
    expect(await screen.findByTestId("topbar")).toHaveClass("md:hidden")
  })

  test("puts the compact generate button next to the gear", async () => {
    renderWithRouter(<TopBar />)
    const bar = await screen.findByTestId("topbar")
    const generate = within(bar).getByTestId("generate-plan-compact")
    expect(generate).toHaveAccessibleName("Generate Plan")
    // Same trailing group as the gear, placed before it.
    const gear = within(bar).getByTestId("topbar-settings")
    expect(generate.parentElement).toBe(gear.parentElement)
    expect(
      generate.compareDocumentPosition(gear) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })
})
