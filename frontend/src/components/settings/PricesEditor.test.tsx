import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { SettingItem } from "@/lib/api/settings"
import { mockPriceInfo, mockSettingsList } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/settings", () => ({
  listSettings: vi.fn(),
  setSetting: vi.fn(),
  clearSetting: vi.fn(),
}))
vi.mock("@/lib/api/prices", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/prices")>()),
  getPrice: vi.fn(),
}))

import { getPrice } from "@/lib/api/prices"
import { listSettings, setSetting } from "@/lib/api/settings"

import { PricesEditor } from "./PricesEditor"

function serveSettings(overrides: Partial<SettingItem>[] = []) {
  const stored = new Map(overrides.map((o) => [o.key!, o]))
  vi.mocked(listSettings).mockImplementation(async () =>
    mockSettingsList([...stored.values()])
  )
  vi.mocked(setSetting).mockImplementation(async (key, value) => {
    stored.set(key, { key, value, source: "db" })
  })
}

describe("PricesEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    serveSettings([{ key: "ai.model", value: "claude-sonnet-5-5" }])
    vi.mocked(getPrice).mockResolvedValue(mockPriceInfo)
  })

  it("shows the configured model's built-in price with its date", async () => {
    renderWithRouter(<PricesEditor />)

    expect(await screen.findByTestId("current-price")).toHaveTextContent(
      "claude-sonnet-5-5: $3.00 in, $15.00 out (built-in, as of Oct 31, 2026)."
    )
    expect(getPrice).toHaveBeenCalledWith("claude-sonnet-5-5")
  })

  it("says when the model has no price", async () => {
    vi.mocked(getPrice).mockResolvedValue({
      model: "fake",
      price: null,
      source: "none",
      as_of: "2026-10-31",
    })
    renderWithRouter(<PricesEditor />)

    expect(await screen.findByTestId("current-price")).toHaveTextContent(
      /fake has no built-in price/
    )
  })

  it("saves a complete override row through settings (F7-AC3)", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PricesEditor />)

    await user.click(await screen.findByTestId("price-override-add"))
    const row = screen.getByTestId("price-override-row")
    await user.type(within(row).getByLabelText("Model id"), "fake")
    await user.type(within(row).getByLabelText("Input"), "2")
    // Incomplete (no output price): nothing saved yet.
    await user.click(within(row).getByLabelText("Output"))
    expect(setSetting).not.toHaveBeenCalled()
    expect(within(row).getByText(/Needs a model id/)).toBeInTheDocument()

    await user.type(within(row).getByLabelText("Output"), "8")
    await user.tab()
    expect(setSetting).toHaveBeenCalledTimes(1)
    expect(setSetting).toHaveBeenCalledWith(
      "ai.price_overrides",
      JSON.stringify({
        fake: { in: 2, out: 8, cache_read: null, cache_write: null },
      })
    )
    expect(await screen.findByText("Saved")).toBeInTheDocument()
  })

  it("loads stored overrides and removes one", async () => {
    serveSettings([
      { key: "ai.model", value: "claude-sonnet-5-5" },
      {
        key: "ai.price_overrides",
        value: JSON.stringify({
          fake: { in: 1, out: 2, cache_read: 0.1, cache_write: null },
        }),
        source: "db",
      },
    ])
    const user = userEvent.setup()
    renderWithRouter(<PricesEditor />)

    const row = await screen.findByTestId("price-override-row")
    expect(within(row).getByLabelText("Model id")).toHaveValue("fake")
    expect(within(row).getByLabelText("Cache read")).toHaveValue(0.1)

    await user.click(
      within(row).getByRole("button", { name: "Remove the price for fake" })
    )
    expect(setSetting).toHaveBeenCalledWith("ai.price_overrides", "{}")
    expect(screen.queryByTestId("price-override-row")).toBeNull()
  })

  it("does not save a negative price", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PricesEditor />)

    await user.click(await screen.findByTestId("price-override-add"))
    const row = screen.getByTestId("price-override-row")
    await user.type(within(row).getByLabelText("Model id"), "m")
    await user.type(within(row).getByLabelText("Input"), "-1")
    await user.type(within(row).getByLabelText("Output"), "1")
    await user.tab()
    expect(setSetting).not.toHaveBeenCalled()
  })

  it("keeps a saved price while its row is incomplete; only Remove deletes", async () => {
    serveSettings([
      { key: "ai.model", value: "claude-sonnet-5-5" },
      {
        key: "ai.price_overrides",
        value: JSON.stringify({
          fake: { in: 1, out: 2, cache_read: null, cache_write: null },
          other: { in: 3, out: 4, cache_read: null, cache_write: null },
        }),
        source: "db",
      },
    ])
    const user = userEvent.setup()
    renderWithRouter(<PricesEditor />)

    const [fake] = await screen.findAllByTestId("price-override-row")
    await user.clear(within(fake).getByLabelText("Input"))
    await user.tab()
    expect(setSetting).not.toHaveBeenCalled()
    expect(within(fake).getByText(/Needs a model id/)).toBeInTheDocument()

    // Removing the other row deletes only its stored price.
    const [, other] = screen.getAllByTestId("price-override-row")
    await user.click(
      within(other).getByRole("button", { name: "Remove the price for other" })
    )
    expect(setSetting).toHaveBeenCalledWith(
      "ai.price_overrides",
      JSON.stringify({
        fake: { in: 1, out: 2, cache_read: null, cache_write: null },
      })
    )
  })

  it("flags a duplicate model id and does not save", async () => {
    serveSettings([
      { key: "ai.model", value: "claude-sonnet-5-5" },
      {
        key: "ai.price_overrides",
        value: JSON.stringify({ fake: { in: 1, out: 2 } }),
        source: "db",
      },
    ])
    const user = userEvent.setup()
    renderWithRouter(<PricesEditor />)

    await user.click(await screen.findByTestId("price-override-add"))
    const [, row] = screen.getAllByTestId("price-override-row")
    await user.type(within(row).getByLabelText("Model id"), "fake")
    await user.type(within(row).getByLabelText("Input"), "5")
    await user.type(within(row).getByLabelText("Output"), "6")
    await user.tab()
    expect(setSetting).not.toHaveBeenCalled()
    expect(within(row).getByLabelText("Model id")).toHaveAttribute(
      "aria-invalid",
      "true"
    )
    expect(
      within(row).getByText(/Another row already prices this model/)
    ).toBeInTheDocument()
  })

  it("says when the price can't be loaded", async () => {
    vi.mocked(getPrice).mockRejectedValue(new Error("boom"))
    renderWithRouter(<PricesEditor />)

    expect(await screen.findByTestId("current-price")).toHaveTextContent(
      "Couldn't load the price for this model."
    )
  })
})
