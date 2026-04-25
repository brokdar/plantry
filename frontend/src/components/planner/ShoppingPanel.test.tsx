import { describe, expect, test, vi, beforeEach, beforeAll } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { ShoppingPanel } from "./ShoppingPanel"

vi.mock("@/lib/queries/weeks", () => ({
  useShoppingList: vi.fn(),
  useWeekNutrition: vi.fn(),
  useCurrentWeek: vi.fn(),
  useWeek: vi.fn(),
  useWeekByDate: vi.fn(),
  useCopyWeek: vi.fn(),
  useCreatePlate: vi.fn(),
}))

import { useShoppingList } from "@/lib/queries/weeks"

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
}

beforeAll(() => {
  vi.stubGlobal("localStorage", localStorageMock)
})

const defaultProps = {
  weekId: 1,
  open: true,
  onOpenChange: vi.fn(),
}

beforeEach(() => {
  vi.clearAllMocks()
  Object.keys(store).forEach((k) => delete store[k])
  // Restore spies on the mock after clearAllMocks resets them.
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

  test("renders item list with name and grams", async () => {
    vi.mocked(useShoppingList).mockReturnValue({
      data: {
        items: [
          { food_id: 1, name: "Chicken", total_grams: 350 },
          { food_id: 2, name: "Rice", total_grams: 200.7 },
        ],
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useShoppingList>)

    renderWithRouter(<ShoppingPanel {...defaultProps} />, "/")
    expect(await screen.findByText("Chicken")).toBeInTheDocument()
    expect(screen.getByText("350 g")).toBeInTheDocument()
    expect(screen.getByText("Rice")).toBeInTheDocument()
    expect(screen.getByText("201 g")).toBeInTheDocument()
  })

  test("checking an item persists to localStorage", async () => {
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

    const stored = JSON.parse(
      localStorage.getItem("plantry:purchased:week:1") ?? "[]"
    ) as number[]
    expect(stored).toContain(5)
  })

  test("unchecking an item removes it from localStorage", async () => {
    const user = userEvent.setup()
    localStorage.setItem("plantry:purchased:week:1", JSON.stringify([5]))
    vi.mocked(useShoppingList).mockReturnValue({
      data: {
        items: [{ food_id: 5, name: "Broccoli", total_grams: 100 }],
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useShoppingList>)

    renderWithRouter(<ShoppingPanel {...defaultProps} />, "/")
    const checkbox = await screen.findByRole("checkbox")
    expect(checkbox).toBeChecked()
    await user.click(checkbox)

    const stored = JSON.parse(
      localStorage.getItem("plantry:purchased:week:1") ?? "[]"
    ) as number[]
    expect(stored).not.toContain(5)
  })
})
