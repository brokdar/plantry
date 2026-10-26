import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"
import { screen, within } from "@testing-library/react"

import { renderWithRouter } from "@/test/render"
import { MobileBottomNav } from "./MobileBottomNav"

describe("MobileBottomNav", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date(2026, 9, 26, 12))
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  test("renders the four tabs", async () => {
    renderWithRouter(<MobileBottomNav />)
    const nav = await screen.findByTestId("mobile-bottom-nav")
    const tabs = within(nav).getAllByRole("link")
    expect(tabs.map((l) => l.textContent)).toEqual([
      "Today",
      "Plan",
      "Recipes",
      "Shop",
    ])
    expect(tabs[0]).toHaveAttribute("href", "/day/2026-10-26")
  })

  test("has exactly four items and no generate FAB (UX §2.1)", async () => {
    renderWithRouter(<MobileBottomNav />)
    const nav = await screen.findByTestId("mobile-bottom-nav")
    expect(within(nav).getAllByRole("link")).toHaveLength(4)
    expect(within(nav).queryByRole("button")).toBeNull()
    expect(nav).toHaveClass("grid-cols-4")
  })

  test("Recipes is active on /ingredients", async () => {
    renderWithRouter(<MobileBottomNav />, "/ingredients")
    const nav = await screen.findByTestId("mobile-bottom-nav")
    expect(within(nav).getByRole("link", { name: "Recipes" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(within(nav).getByRole("link", { name: "Plan" })).not.toHaveAttribute(
      "aria-current"
    )
  })
})
