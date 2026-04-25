import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { mockLookupCandidate, mockLookupResponse } from "@/test/fixtures"
import { LookupPanel } from "./LookupPanel"

vi.mock("@/lib/api/lookup", () => ({
  lookupFoods: vi.fn(),
}))

import { lookupFoods } from "@/lib/api/lookup"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("LookupPanel", () => {
  test("renders search input", async () => {
    const onSelect = vi.fn()
    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    expect(
      await screen.findByPlaceholderText(/search by name or barcode/i)
    ).toBeInTheDocument()
  })

  test("shows candidate detail and source badge after typing a query", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupFoods).mockResolvedValue(mockLookupResponse)

    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "chicken")

    // NutritionDetail renders the candidate name.
    expect(await screen.findByText("Chicken Breast, Raw")).toBeInTheDocument()
    // Source badge ("USDA") appears both inside the detail card and beside
    // the apply button, so match at least one.
    expect(screen.getAllByText("USDA").length).toBeGreaterThan(0)
    // The recommendation marker is a Sparkles icon with accessible label.
    expect(screen.getByLabelText(/recommended/i)).toBeInTheDocument()
  })

  test("clicking Apply calls onSelect with the selected candidate", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupFoods).mockResolvedValue(mockLookupResponse)

    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "chicken")

    const applyButton = await screen.findByRole("button", {
      name: /use this match/i,
    })
    await user.click(applyButton)

    expect(onSelect).toHaveBeenCalledWith(mockLookupCandidate)
  })

  test("shows no results message for empty results", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupFoods).mockResolvedValue({
      results: [],
      recommended_index: -1,
    })

    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "xyznonexistent")

    expect(await screen.findByText(/no matches found/i)).toBeInTheDocument()
  })

  test("shows error message on failure", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupFoods).mockRejectedValue(new Error("Network error"))

    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "chicken")

    await waitFor(() => {
      expect(
        screen.getByText(/could not search external databases/i)
      ).toBeInTheDocument()
    })
  })

  test("renders source_name beneath candidate name when it differs", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupFoods).mockResolvedValue({
      results: [
        {
          ...mockLookupCandidate,
          name: "Paprika",
          source_name: "Spices, paprika",
        },
        {
          ...mockLookupCandidate,
          name: "Paprika Pulver",
          source_name: "Paprika Pulver",
        },
      ],
      recommended_index: 0,
    })

    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "paprika")

    // The differing source_name must appear so the user can disambiguate.
    await screen.findAllByText("Spices, paprika")
    expect(screen.getAllByText("Spices, paprika").length).toBeGreaterThan(0)
  })

  test("routes all-digit input to the barcode lookup path", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupFoods).mockResolvedValue(mockLookupResponse)

    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "3017620422003")

    await waitFor(() => {
      expect(lookupFoods).toHaveBeenCalledWith(
        expect.objectContaining({ barcode: "3017620422003" })
      )
    })
    expect(lookupFoods).not.toHaveBeenCalledWith(
      expect.objectContaining({ query: "3017620422003" })
    )
  })
})
