import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { mockSuggestion } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/discovery", () => ({
  getDailySuggestion: vi.fn(),
  getDiscoveryRun: vi.fn(),
  anotherSuggestion: vi.fn(),
  rejectSuggestion: vi.fn(),
}))
vi.mock("@/lib/api/settings", () => ({ listSettings: vi.fn() }))
const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", async (orig) => ({
  ...(await orig<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigateMock,
}))

import {
  anotherSuggestion,
  getDailySuggestion,
  rejectSuggestion,
} from "@/lib/api/discovery"
import { listSettings, type SettingItem } from "@/lib/api/settings"

import { DailySuggestionSheet, DISMISSED_KEY } from "./DailySuggestionSheet"

const DATE = "2026-11-02"

function settings(values: Record<string, string>) {
  const items: SettingItem[] = Object.entries(values).map(([key, value]) => ({
    key,
    value,
    source: "db",
    is_secret: false,
    env_also_set: false,
  }))
  vi.mocked(listSettings).mockResolvedValue({
    items,
    cipher_available: true,
  })
}

const ON = { "ai.provider": "anthropic", "discovery.daily_enabled": "true" }

beforeEach(async () => {
  vi.clearAllMocks()
  localStorage.clear()
  await i18n.changeLanguage("en")
  vi.mocked(getDailySuggestion).mockResolvedValue({
    kind: "suggestion",
    suggestion: mockSuggestion(),
  })
})

describe("DailySuggestionSheet", () => {
  it("shows the idea when available and enabled", async () => {
    settings(ON)
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    expect(
      await screen.findByRole("heading", { name: "Fesenjan" })
    ).toBeInTheDocument()
    expect(getDailySuggestion).toHaveBeenCalledWith(DATE)
  })

  it("F2-AC2: closing records the dismissal and does not reject", async () => {
    settings(ON)
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    await screen.findByRole("heading", { name: "Fesenjan" })
    await userEvent.click(screen.getByRole("button", { name: "Close" }))
    await waitFor(() =>
      expect(screen.queryByRole("heading", { name: "Fesenjan" })).toBeNull()
    )
    expect(localStorage.getItem(DISMISSED_KEY)).toBe(DATE)
    expect(rejectSuggestion).not.toHaveBeenCalled()
  })

  it("is not shown again on a day it was dismissed", async () => {
    settings(ON)
    localStorage.setItem(DISMISSED_KEY, DATE)
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    await waitFor(() => expect(listSettings).toHaveBeenCalled())
    expect(getDailySuggestion).not.toHaveBeenCalled()
    expect(screen.queryByRole("heading", { name: "Fesenjan" })).toBeNull()
  })

  it("F2-AC3: 204 renders nothing", async () => {
    settings(ON)
    vi.mocked(getDailySuggestion).mockResolvedValue({ kind: "none" })
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    await waitFor(() => expect(getDailySuggestion).toHaveBeenCalled())
    expect(screen.queryByRole("dialog")).toBeNull()
  })

  it("F2-AC3: a failed request or run renders nothing", async () => {
    settings(ON)
    vi.mocked(getDailySuggestion).mockRejectedValue(new Error("500"))
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    await waitFor(() => expect(getDailySuggestion).toHaveBeenCalled())
    expect(screen.queryByRole("dialog")).toBeNull()
    expect(screen.queryByRole("alert")).toBeNull()
  })

  it.each([
    ["no provider", { "discovery.daily_enabled": "true" }],
    [
      "daily disabled",
      { "ai.provider": "anthropic", "discovery.daily_enabled": "false" },
    ],
  ])("F2-AC4: no call when %s", async (_n, values) => {
    settings(values)
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    await waitFor(() => expect(listSettings).toHaveBeenCalled())
    await new Promise((r) => setTimeout(r, 50))
    expect(getDailySuggestion).not.toHaveBeenCalled()
    expect(screen.queryByRole("dialog")).toBeNull()
  })

  it("Not for me: one tap on a reason rejects with it and closes", async () => {
    settings(ON)
    vi.mocked(rejectSuggestion).mockResolvedValue(
      mockSuggestion({ status: "rejected" })
    )
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    await screen.findByRole("heading", { name: "Fesenjan" })
    await userEvent.click(screen.getByRole("button", { name: "Not for me" }))
    await userEvent.click(
      screen.getByRole("button", { name: "Takes too long" })
    )
    await waitFor(() =>
      expect(rejectSuggestion).toHaveBeenCalledWith(5, "too_long")
    )
    await waitFor(() => expect(localStorage.getItem(DISMISSED_KEY)).toBe(DATE))
  })

  it("Add to my recipes opens the prefilled review", async () => {
    settings(ON)
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    await screen.findByRole("heading", { name: "Fesenjan" })
    await userEvent.click(
      screen.getByRole("button", { name: "Add to my recipes" })
    )
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/recipes",
      search: { add: "discover", suggestion: 5 },
    })
  })

  it("Show another swaps in the new idea", async () => {
    settings(ON)
    vi.mocked(anotherSuggestion).mockResolvedValue({
      kind: "suggestion",
      suggestion: mockSuggestion({ id: 6, name: "Ghormeh Sabzi" }),
    })
    renderWithRouter(<DailySuggestionSheet date={DATE} />)
    await screen.findByRole("heading", { name: "Fesenjan" })
    await userEvent.click(screen.getByRole("button", { name: "Show another" }))
    expect(
      await screen.findByRole("heading", { name: "Ghormeh Sabzi" })
    ).toBeInTheDocument()
    expect(anotherSuggestion).toHaveBeenCalledWith(DATE)
  })
})
