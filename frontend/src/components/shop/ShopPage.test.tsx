import { act, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { useState } from "react"

import { ApiError } from "@/lib/api/client"
import type { ShoppingList } from "@/lib/api/shopping"
import i18n from "@/lib/i18n"
import { mockPlan, mockShoppingList } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/shopping", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/shopping")>()),
  getShoppingList: vi.fn(),
}))
vi.mock("@/lib/api/plan", () => ({
  getPlan: vi.fn(),
  clearPlan: vi.fn(),
  restorePlan: vi.fn(),
}))
vi.mock("@/lib/api/foods", () => ({
  getFood: vi.fn(),
  setShopSection: vi.fn(),
}))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { getFood, setShopSection } from "@/lib/api/foods"
import { getPlan } from "@/lib/api/plan"
import { getShoppingList } from "@/lib/api/shopping"

import { ShopPage } from "./ShopPage"

const STORAGE_KEY = "plantry:purchased:range:2026-10-31:2026-11-06"

function renderShop(
  list: ShoppingList = mockShoppingList,
  onRangeChange = vi.fn(),
  includeDrafts = false
) {
  vi.mocked(getShoppingList).mockResolvedValue(list)
  const onIncludeDraftsChange = vi.fn()
  renderWithRouter(
    <ShopPage
      includeDrafts={includeDrafts}
      onRangeChange={onRangeChange}
      onIncludeDraftsChange={onIncludeDraftsChange}
    />
  )
  return { onRangeChange, onIncludeDraftsChange }
}

function row(name: string) {
  return screen.getByText(name).closest("li") as HTMLElement
}

function section(name: string) {
  return screen.getByRole("region", { name })
}

describe("ShopPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    localStorage.clear()
    vi.mocked(getPlan).mockResolvedValue(mockPlan)
    await i18n.changeLanguage("en")
  })

  it("shows the range and the item count in the header", async () => {
    renderShop()
    expect(await screen.findByRole("heading", { level: 1 })).toHaveTextContent(
      "Sat, Oct 31 to Fri, Nov 6"
    )
    expect(screen.getByTestId("shop-count")).toHaveTextContent(
      "2 items · 0 ticked"
    )
  })

  // F7-AC1
  it("lists beans and onions in their sections, used in the batch with its badge", async () => {
    const user = userEvent.setup()
    renderShop()
    await screen.findByText("Onion")

    const produce = section("Produce")
    expect(within(produce).getByText("Onion")).toBeInTheDocument()
    expect(within(produce).getByText("3 pcs")).toBeInTheDocument()
    const cans = section("Cans & jars")
    expect(within(cans).getByText("Kidney beans")).toBeInTheDocument()
    expect(within(cans).getByText("600 g")).toBeInTheDocument()
    // Walk order.
    const headings = screen
      .getAllByRole("heading", { level: 2 })
      .map((h) => h.textContent)
    expect(headings).toEqual([
      expect.stringContaining("Produce"),
      expect.stringContaining("Cans & jars"),
    ])

    await user.click(
      within(cans).getByRole("button", { name: "Where Kidney beans is used" })
    )
    const use = await within(cans).findByTestId("shop-used-in")
    expect(use).toHaveTextContent(/Chili · Sun/)
    expect(use).toHaveTextContent("Nov 1")
    await waitFor(() =>
      expect(use.querySelector("[data-batch-badge]")).toHaveTextContent(
        "Cook × 6"
      )
    )
  })

  // F7-AC2 (the refetch and reload are covered by e2e/shop.spec.ts)
  it("moves an item to another section from the row menu", async () => {
    const user = userEvent.setup()
    vi.mocked(getFood).mockResolvedValue({
      id: 51,
      kind: "leaf",
      shop_section: "produce",
      shop_section_source: "auto",
    } as never)
    vi.mocked(setShopSection).mockResolvedValue({} as never)
    renderShop()
    await screen.findByText("Onion")

    await user.click(screen.getByRole("button", { name: "Options for Onion" }))
    expect(
      screen.getByRole("menuitemradio", { name: "Produce" })
    ).toHaveAttribute("aria-checked", "true")
    await user.click(screen.getByRole("menuitemradio", { name: "Frozen" }))
    expect(setShopSection).toHaveBeenCalledWith(51, "frozen")
  })

  it("says where a moved item went and Undo puts an automatic section back", async () => {
    const user = userEvent.setup()
    vi.mocked(getFood).mockResolvedValue({
      id: 51,
      kind: "leaf",
      shop_section: "produce",
      shop_section_source: "auto",
    } as never)
    vi.mocked(setShopSection).mockResolvedValue({} as never)
    renderShop()
    await screen.findByText("Onion")

    await user.click(screen.getByRole("button", { name: "Options for Onion" }))
    await waitFor(() => expect(getFood).toHaveBeenCalledWith(51))
    await user.click(screen.getByRole("menuitemradio", { name: "Frozen" }))
    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith(
        "Onion moved to Frozen",
        expect.objectContaining({
          action: expect.objectContaining({ label: "Undo" }),
        })
      )
    )
    const [, options] = toastMock.mock.calls[0] as [
      string,
      { action: { onClick: () => void } },
    ]
    options.action.onClick()
    await waitFor(() =>
      expect(setShopSection).toHaveBeenLastCalledWith(51, null)
    )
  })

  it("Undo puts a section the user chose back", async () => {
    const user = userEvent.setup()
    vi.mocked(getFood).mockResolvedValue({
      id: 51,
      kind: "leaf",
      shop_section: "produce",
      shop_section_source: "user",
    } as never)
    vi.mocked(setShopSection).mockResolvedValue({} as never)
    renderShop()
    await screen.findByText("Onion")

    await user.click(screen.getByRole("button", { name: "Options for Onion" }))
    await screen.findByRole("menuitem", { name: "Reset to automatic" })
    await user.click(screen.getByRole("menuitemradio", { name: "Frozen" }))
    await waitFor(() => expect(toastMock).toHaveBeenCalled())
    const [, options] = toastMock.mock.calls[0] as [
      string,
      { action: { onClick: () => void } },
    ]
    options.action.onClick()
    await waitFor(() =>
      expect(setShopSection).toHaveBeenLastCalledWith(51, "produce")
    )
  })

  it("offers Reset to automatic only for a section the user chose", async () => {
    const user = userEvent.setup()
    vi.mocked(getFood).mockResolvedValue({
      id: 51,
      kind: "leaf",
      shop_section: "produce",
      shop_section_source: "user",
    } as never)
    vi.mocked(setShopSection).mockResolvedValue({} as never)
    renderShop()
    await screen.findByText("Onion")

    await user.click(screen.getByRole("button", { name: "Options for Onion" }))
    await user.click(
      await screen.findByRole("menuitem", { name: "Reset to automatic" })
    )
    expect(setShopSection).toHaveBeenCalledWith(51, null)
  })

  it("has no Reset to automatic for an automatic section", async () => {
    const user = userEvent.setup()
    vi.mocked(getFood).mockResolvedValue({
      id: 51,
      kind: "leaf",
      shop_section: "produce",
      shop_section_source: "auto",
    } as never)
    renderShop()
    await screen.findByText("Onion")
    await user.click(screen.getByRole("button", { name: "Options for Onion" }))
    await waitFor(() => expect(getFood).toHaveBeenCalledWith(51))
    expect(
      screen.queryByRole("menuitem", { name: "Reset to automatic" })
    ).toBeNull()
  })

  // F7-AC3
  it("anchors the presets on the viewed window; Next window shifts by 7 days", async () => {
    const user = userEvent.setup()
    const { onRangeChange } = renderShop({
      ...mockShoppingList,
      range: { ...mockShoppingList.range, from: "2026-11-02" },
    })
    await user.click(await screen.findByTestId("shop-range"))

    const thisWindow = screen.getByRole("button", { name: /This window/ })
    // The default range starts today, so no preset is active yet.
    expect(thisWindow).toHaveAttribute("aria-pressed", "false")
    await user.click(screen.getByRole("button", { name: /Next window/ }))
    expect(onRangeChange).toHaveBeenCalledWith({
      from: "2026-11-07",
      to: "2026-11-13",
    })
  })

  it("marks the active preset and applies a custom range", async () => {
    const user = userEvent.setup()
    const { onRangeChange } = renderShop()
    await user.click(await screen.findByTestId("shop-range"))
    expect(screen.getByRole("button", { name: /This window/ })).toHaveAttribute(
      "aria-pressed",
      "true"
    )

    const from = screen.getByLabelText("From")
    await user.clear(from)
    await user.type(from, "2026-11-02")
    await user.click(screen.getByRole("button", { name: "Show list" }))
    expect(onRangeChange).toHaveBeenCalledWith({
      from: "2026-11-02",
      to: "2026-11-06",
    })
  })

  it("caps a custom range at 366 days, as the API does", async () => {
    const user = userEvent.setup()
    renderShop()
    await user.click(await screen.findByTestId("shop-range"))

    const from = screen.getByLabelText("From")
    const to = screen.getByLabelText("To")
    expect(to).toHaveAttribute("max", "2027-11-01")
    expect(from).toHaveAttribute("min", "2025-11-05")

    await user.clear(to)
    await user.type(to, "2027-11-02")
    expect(screen.getByRole("button", { name: "Show list" })).toBeDisabled()
    await user.clear(to)
    await user.type(to, "2027-11-01")
    expect(screen.getByRole("button", { name: "Show list" })).toBeEnabled()
  })

  it("keeps each date of the title on one line", async () => {
    renderShop()
    const heading = await screen.findByRole("heading", { level: 1 })
    // No-break spaces inside each date; the line may break only at "to".
    expect(heading.textContent).toBe("Sat, Oct 31 to Fri, Nov 6")
  })

  // F7-AC4
  it("persists ticks per range, updates the count and sinks ticked rows", async () => {
    const user = userEvent.setup()
    const twoOnions: ShoppingList = {
      ...mockShoppingList,
      sections: [
        {
          section: "produce",
          items: [
            mockShoppingList.sections[0].items[0],
            {
              ...mockShoppingList.sections[0].items[0],
              food_id: 52,
              name: "Garlic",
            },
          ],
        },
        mockShoppingList.sections[1],
      ],
      count: 3,
    }
    renderShop(twoOnions)
    await screen.findByText("Onion")

    await user.click(screen.getByRole("checkbox", { name: /Onion/ }))
    expect(localStorage.getItem(STORAGE_KEY)).toBe("[51]")
    expect(screen.getByTestId("shop-count")).toHaveTextContent(
      "3 items · 1 ticked"
    )
    const rows = within(section("Produce")).getAllByTestId("shop-row")
    expect(rows.map((r) => r.dataset.foodId)).toEqual(["52", "51"])
    expect(row("Onion")).toHaveAttribute("data-ticked", "true")
    expect(within(row("Onion")).getByText("Onion")).toHaveClass("line-through")
  })

  it("restores ticks for the range from localStorage, ignoring ids no longer listed", async () => {
    localStorage.setItem(STORAGE_KEY, "[50, 999]")
    renderShop()
    expect(
      await screen.findByRole("checkbox", { name: /Kidney beans/ })
    ).toBeChecked()
    expect(screen.getByRole("checkbox", { name: /Onion/ })).not.toBeChecked()
    expect(screen.getByTestId("shop-count")).toHaveTextContent(
      "2 items · 1 ticked"
    )
  })

  it("says All done when everything is ticked, and Clear ticked unticks with undo", async () => {
    const user = userEvent.setup()
    localStorage.setItem(STORAGE_KEY, "[50, 51]")
    renderShop()
    expect(await screen.findByTestId("shop-all-done")).toHaveTextContent(
      "All done. 2 items bought."
    )

    await user.click(screen.getByRole("button", { name: "Clear ticked" }))
    expect(localStorage.getItem(STORAGE_KEY)).toBe("[]")
    expect(screen.queryByTestId("shop-all-done")).toBeNull()
    expect(screen.queryByRole("button", { name: "Clear ticked" })).toBeNull()

    const [, options] = toastMock.mock.calls[0] as [
      string,
      { action: { onClick: () => void } },
    ]
    options.action.onClick()
    await waitFor(() =>
      expect(screen.getByRole("checkbox", { name: /Onion/ })).toBeChecked()
    )
  })

  // F7-AC5 (page part; the text itself is unit-tested in CopyAsText.test.ts)
  it("copies the list as text", async () => {
    const user = userEvent.setup()
    const writeText = vi
      .spyOn(navigator.clipboard, "writeText")
      .mockResolvedValue()
    renderShop()
    await user.click(
      await screen.findByRole("button", { name: "Copy as text" })
    )
    expect(writeText).toHaveBeenCalledWith(
      "Produce\n- Onion, 3 pcs\n\nCans & jars\n- Kidney beans, 600 g"
    )
    expect(toastMock.success).toHaveBeenCalledWith("List copied")
  })

  it("switches to in-store mode with larger rows and actions at the bottom", async () => {
    const user = userEvent.setup()
    renderShop()
    const toggle = await screen.findByRole("button", { name: "In-store mode" })
    await user.click(toggle)
    expect(toggle).toHaveAttribute("aria-pressed", "true")
    expect(screen.getByText("Onion").closest("label")).toHaveClass("min-h-14")
    await user.click(
      screen.getByRole("button", { name: "Leave in-store mode" })
    )
    expect(toggle).toHaveAttribute("aria-pressed", "false")
  })

  it("keeps the screen on while in-store mode is on", async () => {
    const release = vi.fn().mockResolvedValue(undefined)
    const request = vi.fn().mockResolvedValue({ released: false, release })
    Object.defineProperty(navigator, "wakeLock", {
      value: { request },
      configurable: true,
    })
    try {
      const user = userEvent.setup()
      renderShop()
      const toggle = await screen.findByRole("button", {
        name: "In-store mode",
      })
      expect(request).not.toHaveBeenCalled()

      await user.click(toggle)
      expect(request).toHaveBeenCalledWith("screen")
      await waitFor(() => expect(release).not.toHaveBeenCalled())

      await user.click(
        screen.getByRole("button", { name: "Leave in-store mode" })
      )
      await waitFor(() => expect(release).toHaveBeenCalledTimes(1))
    } finally {
      delete (navigator as { wakeLock?: unknown }).wakeLock
    }
  })

  it("sends an empty range to the plan", async () => {
    renderShop({ ...mockShoppingList, sections: [], count: 0 })
    expect(
      await screen.findByText("Plan this window to see what to buy")
    ).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Go to plan" })).toHaveAttribute(
      "href",
      "/plan?start=2026-10-31"
    )
    expect(screen.getByTestId("shop-count")).toHaveTextContent("0 items")
  })

  // Phase 3 F5-AC3: no drafts in the range, no toggle.
  it("hides the draft toggle when the range has no drafts", async () => {
    renderShop()
    await screen.findByText("Onion")
    expect(getShoppingList).toHaveBeenCalledWith(undefined, false)
    expect(
      screen.queryByRole("switch", { name: "Include draft plates" })
    ).toBeNull()
  })

  // Phase 3 F5: with drafts in the range the toggle shows and reports the
  // change.
  it("shows the draft toggle when the range has drafts and switches it on", async () => {
    const user = userEvent.setup()
    const { onIncludeDraftsChange } = renderShop({
      ...mockShoppingList,
      has_drafts: true,
    })
    const toggle = await screen.findByRole("switch", {
      name: "Include draft plates",
    })
    expect(toggle).not.toBeChecked()
    await user.click(toggle)
    expect(onIncludeDraftsChange).toHaveBeenCalledWith(true)
  })

  // Phase 3 F5: a range with only drafts lists nothing confirmed, yet the
  // toggle stays reachable above the empty state.
  it("asks for drafts while on and keeps the toggle on an empty list", async () => {
    const user = userEvent.setup()
    const { onIncludeDraftsChange } = renderShop(
      { ...mockShoppingList, sections: [], count: 0, has_drafts: true },
      vi.fn(),
      true
    )
    const toggle = await screen.findByRole("switch", {
      name: "Include draft plates",
    })
    expect(toggle).toBeChecked()
    expect(getShoppingList).toHaveBeenCalledWith(undefined, true)
    expect(screen.getByTestId("shop-empty")).toBeInTheDocument()
    await user.click(toggle)
    expect(onIncludeDraftsChange).toHaveBeenCalledWith(false)
  })

  // The page owns the toggle as the route does: flipping it keeps the last
  // list on screen while the new one loads, so the switch keeps focus.
  function DraftsHarness() {
    const [on, setOn] = useState(false)
    return (
      <ShopPage
        includeDrafts={on}
        onRangeChange={vi.fn()}
        onIncludeDraftsChange={setOn}
      />
    )
  }

  it("keeps the list and the switch's focus while the draft list loads", async () => {
    const user = userEvent.setup()
    vi.mocked(getShoppingList).mockImplementation((_range, drafts) =>
      drafts
        ? new Promise(() => {})
        : Promise.resolve({ ...mockShoppingList, has_drafts: true })
    )
    renderWithRouter(<DraftsHarness />)
    const toggle = await screen.findByRole("switch", {
      name: "Include draft plates",
    })
    await user.click(toggle)
    await waitFor(() =>
      expect(getShoppingList).toHaveBeenCalledWith(undefined, true)
    )
    expect(screen.queryByTestId("shop-skeleton")).toBeNull()
    const after = screen.getByRole("switch", { name: "Include draft plates" })
    expect(after).toBe(toggle)
    expect(after).toBeChecked()
    expect(after).toHaveFocus()
    expect(screen.getByText("Onion")).toBeInTheDocument()
  })

  // Ticks of items only on drafts survive while drafts are hidden.
  it("keeps saved ticks of items not on the list when ticking and clearing", async () => {
    const user = userEvent.setup()
    localStorage.setItem(STORAGE_KEY, "[999]")
    renderShop()
    await user.click(await screen.findByRole("checkbox", { name: /Onion/ }))
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!)).toEqual([999, 51])
    await user.click(screen.getByRole("checkbox", { name: /Onion/ }))
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!)).toEqual([999])

    await user.click(screen.getByRole("checkbox", { name: /Onion/ }))
    await user.click(screen.getByRole("button", { name: "Clear ticked" }))
    expect(localStorage.getItem(STORAGE_KEY)).toBe("[]")
    const undo = toastMock.mock.calls.at(-1)?.[1]?.action as {
      onClick: () => void
    }
    act(() => undo.onClick())
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!)).toEqual([999, 51])
  })

  it("offers a retry when the list fails to load", async () => {
    vi.mocked(getShoppingList).mockRejectedValue(new Error("down"))
    renderWithRouter(
      <ShopPage
        includeDrafts={false}
        onRangeChange={vi.fn()}
        onIncludeDraftsChange={vi.fn()}
      />
    )
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The shopping list didn't load"
    )
    expect(
      screen.getByRole("button", { name: "Try again" })
    ).toBeInTheDocument()
  })

  it("names a refused range and keeps the range picker to choose another", async () => {
    const user = userEvent.setup()
    vi.mocked(getShoppingList).mockRejectedValue(
      new ApiError(400, "error.date_range_too_large")
    )
    const onRangeChange = vi.fn()
    renderWithRouter(
      <ShopPage
        range={{ from: "2026-01-01", to: "2027-06-30" }}
        includeDrafts={false}
        onRangeChange={onRangeChange}
        onIncludeDraftsChange={vi.fn()}
      />
    )
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "That range is longer than a year. Pick a shorter one."
    )
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull()

    await user.click(await screen.findByTestId("shop-range"))
    await user.click(screen.getByRole("button", { name: /This window/ }))
    expect(onRangeChange).toHaveBeenCalledWith({
      from: "2026-10-31",
      to: "2026-11-06",
    })
  })
})
