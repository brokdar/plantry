import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { mockDiscoveryRun, mockSuggestion } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/discovery", () => ({
  discover: vi.fn(),
  getDiscoveryRun: vi.fn(),
  listSuggestions: vi.fn(),
  rejectSuggestion: vi.fn(),
}))
const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", async (orig) => ({
  ...(await orig<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigateMock,
}))

import {
  discover,
  getDiscoveryRun,
  listSuggestions,
  rejectSuggestion,
} from "@/lib/api/discovery"

import { DiscoverSheet } from "./DiscoverSheet"

const onClose = vi.fn()

async function renderSheet() {
  renderWithRouter(<DiscoverSheet open onClose={onClose} />)
  await screen.findByRole("dialog", { name: "Discover recipes" })
  return userEvent.setup()
}

const ideas = [
  mockSuggestion({ id: 1, name: "Pad Thai", known_share: 0.2 }),
  mockSuggestion({ id: 2, name: "Chana Masala", known_share: 0.9 }),
  mockSuggestion({ id: 3, name: "Hummus Bowl", known_share: 0.6 }),
  mockSuggestion({ id: 4, name: "Falafel", known_share: 0.4 }),
]

const cardNames = () =>
  screen
    .getAllByTestId("suggestion-card")
    .map((card) => within(card).getByRole("heading", { level: 3 }).textContent)

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(discover).mockResolvedValue({ run_id: 21 })
  vi.mocked(getDiscoveryRun).mockResolvedValue(mockDiscoveryRun("ok"))
  vi.mocked(listSuggestions).mockResolvedValue({ items: ideas })
})

describe("DiscoverSheet", () => {
  it("runs a cuisine search, shows progress, then at most 3 cards", async () => {
    vi.mocked(getDiscoveryRun)
      .mockResolvedValueOnce(mockDiscoveryRun("running"))
      .mockResolvedValue(mockDiscoveryRun("ok"))
    const user = await renderSheet()

    await user.click(screen.getByRole("button", { name: "Explore a cuisine" }))
    const find = screen.getByRole("button", { name: "Find recipes" })
    // Explore needs a cuisine.
    expect(find).toBeDisabled()
    await user.type(screen.getByLabelText("Cuisine"), "Thai")
    await user.click(find)

    expect(discover).toHaveBeenCalledWith(
      expect.objectContaining({ mode: "explore", cuisine: "Thai" })
    )
    expect(
      await screen.findByText("Looking for recipes. This can take a minute.")
    ).toBeVisible()
    // The run is polled every second, so allow more than one poll.
    await waitFor(
      () => expect(screen.getAllByTestId("suggestion-card")).toHaveLength(3),
      { timeout: 3000 }
    )
    expect(listSuggestions).toHaveBeenCalledWith({ run_id: 21 })
    // Other modes keep the run's order.
    expect(cardNames()).toEqual(["Pad Thai", "Chana Masala", "Hummus Bowl"])
  })

  it("ranks the cards by known-ingredient share in Use what I know", async () => {
    const user = await renderSheet()

    await user.click(screen.getByRole("button", { name: "Use what I know" }))
    await user.type(screen.getByLabelText(/focus ingredient/i), "chickpeas")
    await user.click(screen.getByRole("button", { name: "Find recipes" }))

    expect(discover).toHaveBeenCalledWith(
      expect.objectContaining({ mode: "known", focus: "chickpeas" })
    )
    await waitFor(() => expect(cardNames()).toHaveLength(3))
    expect(cardNames()).toEqual(["Chana Masala", "Hummus Bowl", "Falafel"])
  })

  it("Add to my recipes opens the prefilled review of that idea", async () => {
    const user = await renderSheet()
    await user.click(screen.getByRole("button", { name: "Find recipes" }))
    expect(discover).toHaveBeenCalledWith(
      expect.objectContaining({ mode: "daily" })
    )

    const cards = await screen.findAllByTestId("suggestion-card")
    await user.click(
      within(cards[1].parentElement!).getByRole("button", {
        name: "Add to my recipes",
      })
    )
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/recipes",
      search: { add: "discover", suggestion: 2 },
    })
  })

  it("Not for me rejects that idea with the picked reason", async () => {
    vi.mocked(rejectSuggestion).mockResolvedValue(
      mockSuggestion({ id: 1, status: "rejected", reason: "too_long" })
    )
    const user = await renderSheet()
    await user.click(screen.getByRole("button", { name: "Find recipes" }))

    const cards = await screen.findAllByTestId("suggestion-card")
    const first = within(cards[0].parentElement!)
    await user.click(first.getByRole("button", { name: "Not for me" }))
    await user.click(first.getByRole("button", { name: "Takes too long" }))
    await waitFor(() =>
      expect(rejectSuggestion).toHaveBeenCalledWith(1, "too_long")
    )
  })

  it("says so when no idea passed the checks", async () => {
    vi.mocked(listSuggestions).mockResolvedValue({ items: [] })
    const user = await renderSheet()
    await user.click(screen.getByRole("button", { name: "Find recipes" }))
    expect(
      await screen.findByText(/no recipe passed the checks/i)
    ).toBeVisible()
  })

  it("shows a failed run's error", async () => {
    vi.mocked(getDiscoveryRun).mockResolvedValue(mockDiscoveryRun("error"))
    const user = await renderSheet()
    await user.click(screen.getByRole("button", { name: "Find recipes" }))
    expect(await screen.findByRole("alert")).toBeVisible()
    expect(listSuggestions).not.toHaveBeenCalled()
  })

  it("a failed ideas query shows the error with Retry, not 'running' forever", async () => {
    vi.mocked(listSuggestions)
      .mockRejectedValueOnce(new Error("down"))
      .mockResolvedValue({ items: ideas })
    const user = await renderSheet()
    await user.click(screen.getByRole("button", { name: "Find recipes" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The search didn't work"
    )
    expect(screen.queryByText(/looking for recipes/i)).toBeNull()
    await user.click(screen.getByTestId("discover-retry"))
    expect(await screen.findByTestId("discover-results")).toBeVisible()
  })
})
