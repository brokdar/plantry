import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
  vi,
} from "vitest"
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
  open: true,
  onOpenChange: vi.fn(),
}

function mockEmptyList() {
  vi.mocked(useShoppingList).mockReturnValue({
    data: { items: [] },
    isLoading: false,
  } as unknown as ReturnType<typeof useShoppingList>)
}

afterEach(() => {
  vi.useRealTimers()
})

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

    expect(
      screen.getByRole("button", { name: "This window" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Next window" })
    ).toBeInTheDocument()
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

  describe("presets anchored on the viewed window", () => {
    // Today: Wednesday 2026-10-28.
    beforeEach(() => {
      vi.useFakeTimers({ toFake: ["Date"] })
      vi.setSystemTime(new Date(2026, 9, 28, 12, 0, 0))
      mockEmptyList()
    })

    test("viewing next week's window defaults to that window's start, not today", async () => {
      const range = { from: "2026-11-02", to: "2026-11-08" }
      renderWithRouter(<ShoppingPanel {...defaultProps} range={range} />, "/")
      await screen.findByText(/No ingredients/)

      expect(useShoppingList).toHaveBeenLastCalledWith(
        "2026-11-02",
        "2026-11-08"
      )
      expect(
        screen.getByRole("button", { name: "This window" })
      ).toHaveAttribute("aria-pressed", "true")
      expect(
        screen.queryByRole("button", { name: "Rest of this window" })
      ).not.toBeInTheDocument()
    })

    test("viewing the current window on its third day defaults to today", async () => {
      const range = { from: "2026-10-26", to: "2026-11-01" }
      renderWithRouter(<ShoppingPanel {...defaultProps} range={range} />, "/")
      await screen.findByText(/No ingredients/)

      expect(useShoppingList).toHaveBeenLastCalledWith(
        "2026-10-28",
        "2026-11-01"
      )
      expect(
        screen.getByRole("button", { name: "Rest of this window" })
      ).toHaveAttribute("aria-pressed", "true")
    })

    test("'Next window' selects the following 7 days", async () => {
      const user = userEvent.setup()
      const range = { from: "2026-10-26", to: "2026-11-01" }
      renderWithRouter(<ShoppingPanel {...defaultProps} range={range} />, "/")
      await user.click(
        await screen.findByRole("button", { name: "Next window" })
      )

      expect(useShoppingList).toHaveBeenLastCalledWith(
        "2026-11-02",
        "2026-11-08"
      )
      expect(
        screen.getByRole("button", { name: "Next window" })
      ).toHaveAttribute("aria-pressed", "true")
    })
  })
})
