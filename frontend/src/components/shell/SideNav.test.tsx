import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"
import { screen, within } from "@testing-library/react"

import { renderWithRouter } from "@/test/render"
import { SideNav } from "./SideNav"

function stubMatchMedia(matches: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockImplementation((query: string) => ({
      matches,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
  )
}

describe("SideNav", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date(2026, 9, 26, 12))
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  test.each([true, false])(
    "renders exactly four destinations plus settings (wide=%s)",
    async (isLg) => {
      stubMatchMedia(isLg)
      renderWithRouter(<SideNav collapsed={false} onToggle={vi.fn()} />)

      const primary = await screen.findByRole("navigation", {
        name: /primary/i,
      })
      const links = within(primary).getAllByRole("link")
      expect(links.map((l) => l.textContent)).toEqual([
        "Today",
        "Plan",
        "Recipes",
        "Shop",
      ])
      expect(links.map((l) => l.getAttribute("href"))).toEqual([
        "/today",
        "/plan",
        "/recipes",
        "/shop",
      ])

      const settings = screen.getByRole("link", { name: /settings/i })
      expect(settings).toHaveAttribute("href", "/settings")
      expect(primary).not.toContainElement(settings)
    }
  )

  test.each([
    ["/recipes/ingredients", "Recipes"],
    ["/recipes/ingredients/12/edit", "Recipes"],
    ["/recipes/3", "Recipes"],
    ["/plan", "Plan"],
    ["/today", "Today"],
    ["/today?date=2026-10-20", "Today"],
    ["/shop", "Shop"],
  ])("marks the right destination active on %s", async (path, active) => {
    stubMatchMedia(true)
    renderWithRouter(<SideNav collapsed={false} onToggle={vi.fn()} />, path)

    const primary = await screen.findByRole("navigation", { name: /primary/i })
    const current = within(primary)
      .getAllByRole("link")
      .filter((l) => l.getAttribute("aria-current") === "page")
    expect(current.map((l) => l.textContent)).toEqual([active])
  })

  test("the brand is a link, not a page heading", async () => {
    stubMatchMedia(true)
    renderWithRouter(<SideNav collapsed={false} onToggle={vi.fn()} />)
    const brand = await screen.findByTestId("sidenav-brand")
    expect(within(brand).queryByRole("heading", { level: 1 })).toBeNull()
  })

  test("calendar and presets are no longer destinations", async () => {
    stubMatchMedia(true)
    renderWithRouter(<SideNav collapsed={false} onToggle={vi.fn()} />)
    await screen.findByRole("navigation", { name: /primary/i })
    expect(screen.queryByRole("link", { name: /calendar/i })).toBeNull()
    expect(screen.queryByRole("link", { name: /presets/i })).toBeNull()
  })
})
