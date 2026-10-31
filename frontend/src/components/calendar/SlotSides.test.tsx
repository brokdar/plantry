import { render } from "@testing-library/react"
import { describe, expect, test } from "vitest"

import "@/lib/i18n"
import { SlotSides, type SlotSideItem } from "./SlotSides"

function side(over: Partial<SlotSideItem> = {}): SlotSideItem {
  return {
    name: "Salad",
    role: "side_veg",
    imagePath: null,
    ...over,
  }
}

describe("SlotSides", () => {
  test("renders nothing for an empty list", () => {
    const { container } = render(<SlotSides items={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  test("renders one thumb per item up to the max", () => {
    const { getAllByRole } = render(
      <SlotSides
        items={[
          side({ name: "Rice" }),
          side({ name: "Salad" }),
          side({ name: "Bread" }),
        ]}
      />
    )
    expect(getAllByRole("listitem")).toHaveLength(3)
  })

  test("collapses the tail into a +N badge", () => {
    const { getByText, getAllByRole } = render(
      <SlotSides
        items={[
          side({ name: "A" }),
          side({ name: "B" }),
          side({ name: "C" }),
          side({ name: "D" }),
          side({ name: "E" }),
        ]}
        max={3}
      />
    )
    expect(getAllByRole("listitem")).toHaveLength(3)
    expect(getByText("+2")).toBeInTheDocument()
  })

  test("group aria-label lists every side name for screen readers", () => {
    const { getByRole } = render(
      <SlotSides items={[side({ name: "Rice" }), side({ name: "Salad" })]} />
    )
    const list = getByRole("list")
    expect(list).toHaveAttribute("aria-label", expect.stringContaining("Rice"))
    expect(list).toHaveAttribute("aria-label", expect.stringContaining("Salad"))
  })
})
