import { describe, expect, test, vi, beforeEach, beforeAll } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { ShoppingPanel } from "./ShoppingPanel"

vi.mock("@/lib/queries/shopping", () => ({
  useShoppingList: vi.fn(),
}))

import { useShoppingList } from "@/lib/queries/shopping"

// Stub localStorage for the test environment.
const store: Record<string, string> = {}
const localStorageMock = {
  getItem: vi.fn((key: string) => store[key] ?? null),
  setItem: vi.fn((key: string, value: string) => {
    store[key] = value
  }),
  removeItem: vi.fn((key: string) => {
    delete store[key]
  }),
  length: 0,
  key: vi.fn(() => null),
  clear: vi.fn(),
}

beforeAll(() => {
  vi.stubGlobal("localStorage", localStorageMock)
})

const defaultRange = { from: "2026-04-26", to: "2026-05-02" }
const defaultProps = {
  range: defaultRange,
  shoppingDay: 6, // Saturday
  open: true,
  onOpenChange: vi.fn(),
}

beforeEach(() => {
  vi.clearAllMocks()
  Object.keys(store).forEach((k) => delete store[k])
  localStorageMock.getItem.mockImplementation(
    (key: string) => store[key] ?? null
  )
  localStorageMock.setItem.mockImplementation((key: string, value: string) => {
    store[key] = value
  })
  localStorageMock.removeItem.mockImplementation((key: string) => {
    delete store[key]
  })
})

describe("ShoppingPanel", () => {
  test("renders loading state", async () => {
    vi.mocked(useShoppingList).mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useShoppingList>)

    renderWithRouter(<ShoppingPanel {...defaultProps} />, "/")
    expect(await screen.findByText("Loading...")).toBeInTheDocument()
  })

  test("renders empty state when no items", async () => {
    vi.mocked(useShoppingList).mockReturnValue({
      data: { items: [] },
      isLoading: false,
    } as unknown as ReturnType<typeof useShoppingList>)

    renderWithRouter(<ShoppingPanel {...defaultProps} />, "/")
    expect(await screen.findByText(/No ingredients/)).toBeInTheDocument()
  })

  test("calls useShoppingList with from/to range", async () => {
    vi.mocked(useShoppingList).mockReturnValue({
      data: { items: [] },
      isLoading: false,
    } as unknown as ReturnType<typeof useShoppingList>)

    renderWithRouter(<ShoppingPanel {...defaultProps} />, "/")
    await screen.findByText(/No ingredients/)

    expect(useShoppingList).toHaveBeenCalledWith(
      defaultRange.from,
      defaultRange.to
    )
  })

  test("renders preset chips", async () => {
    vi.mocked(useShoppingList).mockReturnValue({
      data: { items: [] },
      isLoading: false,
    } as unknown as ReturnType<typeof useShoppingList>)

    renderWithRouter(<ShoppingPanel {...defaultProps} />, "/")
    await screen.findByText(/No ingredients/)

    // All three preset buttons should be rendered.
    expect(screen.getByText(/Next 7 days/i)).toBeInTheDocument()
  })

  test("localStorage key uses range:{from}:{to}", async () => {
    const user = userEvent.setup()
    vi.mocked(useShoppingList).mockReturnValue({
      data: {
        items: [{ food_id: 5, name: "Broccoli", total_grams: 100 }],
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useShoppingList>)

    renderWithRouter(<ShoppingPanel {...defaultProps} />, "/")
    const checkbox = await screen.findByRole("checkbox")
    await user.click(checkbox)

    const expectedKey = `plantry:purchased:range:${defaultRange.from}:${defaultRange.to}`
    expect(localStorageMock.setItem).toHaveBeenCalledWith(
      expectedKey,
      expect.any(String)
    )
  })

  test("migrates old purchased:week: keys on mount", async () => {
    store["plantry:purchased:week:42"] = JSON.stringify([1, 2])
    localStorageMock.length = 1
    localStorageMock.key.mockReturnValue(
      "plantry:purchased:week:42" as unknown as null
    )

    vi.mocked(useShoppingList).mockReturnValue({
      data: { items: [] },
      isLoading: false,
    } as unknown as ReturnType<typeof useShoppingList>)

    renderWithRouter(<ShoppingPanel {...defaultProps} />, "/")
    await screen.findByText(/No ingredients/)

    expect(localStorageMock.removeItem).toHaveBeenCalledWith(
      "plantry:purchased:week:42"
    )
    expect(localStorageMock.setItem).toHaveBeenCalledWith(
      "plantry:migrated:v4-shopping",
      "1"
    )
  })
})
