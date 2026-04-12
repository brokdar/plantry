import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import type { LookupCandidate, LookupResponse } from "@/lib/api/lookup"
import { LookupPanel } from "./LookupPanel"

vi.mock("@/lib/api/lookup", () => ({
  lookupIngredients: vi.fn(),
}))

import { lookupIngredients } from "@/lib/api/lookup"

const mockCandidate: LookupCandidate = {
  name: "Chicken Breast, Raw",
  source: "fdc",
  barcode: null,
  fdc_id: 171077,
  image_url: null,
  existing_id: null,
  kcal_100g: 120,
  protein_100g: 22.5,
  fat_100g: 2.6,
  carbs_100g: 0,
  fiber_100g: 0,
  sodium_100g: 0.074,
  portions: [],
}

const mockResponse: LookupResponse = {
  results: [mockCandidate],
  recommended_index: 0,
}

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

  test("shows candidates after typing a query", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupIngredients).mockResolvedValue(mockResponse)

    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "chicken")

    expect(await screen.findByText("Chicken Breast, Raw")).toBeInTheDocument()
    expect(screen.getByText("Recommended")).toBeInTheDocument()
    expect(screen.getByText("USDA")).toBeInTheDocument()
  })

  test("click candidate calls onSelect", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupIngredients).mockResolvedValue(mockResponse)

    renderWithRouter(<LookupPanel onSelect={onSelect} />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "chicken")

    const candidateButton = await screen.findByText("Chicken Breast, Raw")
    await user.click(candidateButton)

    expect(onSelect).toHaveBeenCalledWith(mockCandidate)
  })

  test("shows no results message for empty results", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    vi.mocked(lookupIngredients).mockResolvedValue({
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
    vi.mocked(lookupIngredients).mockRejectedValue(new Error("Network error"))

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
})
