import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/themes", () => ({
  getWindowThemes: vi.fn(),
  setWindowTheme: vi.fn(),
  removeWindowTheme: vi.fn(),
  acceptThemeSuggestion: vi.fn(),
  declineThemeSuggestion: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({
  toast: Object.assign(vi.fn(), { success: vi.fn() }),
  toastError: vi.fn(),
}))

import {
  acceptThemeSuggestion,
  declineThemeSuggestion,
  getWindowThemes,
  removeWindowTheme,
  setWindowTheme,
} from "@/lib/api/themes"
import { toast } from "@/lib/toast"

import { ThemeBar } from "./ThemeBar"

const japanese = {
  id: 3,
  kind: "cuisine" as const,
  value: "japanese",
  source: "user" as const,
  whole_week: false,
}

describe("ThemeBar", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
  })

  it("adds a theme and says plainly when the library has none", async () => {
    vi.mocked(getWindowThemes).mockResolvedValue({
      themes: [],
      again: [],
      suggestion: null,
    })
    vi.mocked(setWindowTheme).mockResolvedValue({
      theme: japanese,
      library: 0,
      finding: 2,
    })
    const user = userEvent.setup()
    renderWithRouter(<ThemeBar start="2026-11-28" enabled />)
    await user.click(await screen.findByRole("button", { name: "Add theme" }))
    await user.click(screen.getByRole("radio", { name: "Cuisine" }))
    await user.type(screen.getByRole("textbox", { name: "Theme" }), "Japanese")
    await user.click(screen.getByRole("button", { name: "Add" }))
    await waitFor(() =>
      expect(setWindowTheme).toHaveBeenCalledWith("2026-11-28", {
        kind: "cuisine",
        value: "Japanese",
      })
    )
    expect(toast.success).toHaveBeenCalledWith(
      "No Japanese recipes yet. I'll find 2."
    )
  })

  it("removes a theme and hides Add at two themes", async () => {
    vi.mocked(getWindowThemes).mockResolvedValue({
      themes: [
        japanese,
        { ...japanese, id: 4, kind: "mood", value: "one-pot" },
      ],
      again: [],
      suggestion: null,
    })
    vi.mocked(removeWindowTheme).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderWithRouter(<ThemeBar start="2026-11-28" enabled />)
    await user.click(
      await screen.findByRole("button", { name: "Remove theme Japanese" })
    )
    expect(removeWindowTheme).toHaveBeenCalledWith("2026-11-28", 3)
    expect(screen.queryByRole("button", { name: "Add theme" })).toBeNull()
  })

  it("offers last week's theme again with one tap", async () => {
    vi.mocked(getWindowThemes).mockResolvedValue({
      themes: [],
      again: [japanese],
      suggestion: null,
    })
    vi.mocked(setWindowTheme).mockResolvedValue({
      theme: japanese,
      library: 4,
      finding: 0,
    })
    const user = userEvent.setup()
    renderWithRouter(<ThemeBar start="2026-11-28" enabled />)
    expect(await screen.findByText(/Last week: Japanese/)).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Again?" }))
    expect(setWindowTheme).toHaveBeenCalledWith("2026-11-28", {
      kind: "cuisine",
      value: "japanese",
      source: "again",
    })
  })

  it("accepts or declines the suggestion, never applying it alone", async () => {
    vi.mocked(getWindowThemes).mockResolvedValue({
      themes: [],
      again: [],
      suggestion: { kind: "cuisine", value: "thai", state: "open" },
    })
    vi.mocked(declineThemeSuggestion).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderWithRouter(<ThemeBar start="2026-11-28" enabled />)
    expect(
      await screen.findByText("How about Thai this week?")
    ).toBeInTheDocument()
    expect(acceptThemeSuggestion).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: "No thanks" }))
    expect(declineThemeSuggestion).toHaveBeenCalledWith("2026-11-28")
  })

  it("shows nothing with the Chef off", () => {
    const { container } = renderWithRouter(
      <ThemeBar start="2026-11-28" enabled={false} />
    )
    expect(getWindowThemes).not.toHaveBeenCalled()
    expect(container.querySelector("[data-testid=theme-bar]")).toBeNull()
  })
})
