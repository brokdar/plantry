import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"
import "@/lib/i18n"

import type { LeafFood } from "@/lib/api/foods"

import { QuantityUnitInput } from "./QuantityUnitInput"

const apple: LeafFood = {
  id: 1,
  kind: "leaf",
  name: "Apple",
  source: "manual",
  barcode: null,
  off_id: null,
  fdc_id: null,
  image_path: null,
  favorite: false,
  cook_count: 0,
  kcal_100g: 52,
  protein_100g: 0.3,
  fat_100g: 0.2,
  carbs_100g: 14,
  fiber_100g: 2.4,
  sodium_100g: 0,
  created_at: "",
  updated_at: "",
  portions: [{ food_id: 1, unit: "apple", grams: 180 }],
}

const rice: LeafFood = {
  id: 2,
  kind: "leaf",
  name: "Rice",
  source: "manual",
  barcode: null,
  off_id: null,
  fdc_id: null,
  image_path: null,
  favorite: false,
  cook_count: 0,
  kcal_100g: 130,
  protein_100g: 2.7,
  fat_100g: 0.3,
  carbs_100g: 28,
  fiber_100g: 0.4,
  sodium_100g: 0,
  created_at: "",
  updated_at: "",
}

describe("QuantityUnitInput defaults", () => {
  it("default unit = first entry from food's portions", () => {
    const onChange = vi.fn()
    render(<QuantityUnitInput food={apple} onChange={onChange} />)
    // Initial emit fires from a useEffect on first render.
    expect(onChange).toHaveBeenCalled()
    const last = onChange.mock.calls.at(-1)![0]
    expect(last.unit).toBe("apple")
    expect(last.amount).toBe(1)
  })

  it("default unit falls back to g when food has no portions", () => {
    const onChange = vi.fn()
    render(<QuantityUnitInput food={rice} onChange={onChange} />)
    expect(onChange).toHaveBeenCalled()
    const last = onChange.mock.calls.at(-1)![0]
    expect(last.unit).toBe("g")
    expect(last.amount).toBe(100)
  })

  it("recentUnit overrides the default when set", () => {
    const onChange = vi.fn()
    render(
      <QuantityUnitInput food={apple} recentUnit="g" onChange={onChange} />
    )
    const last = onChange.mock.calls.at(-1)![0]
    expect(last.unit).toBe("g")
  })
})

describe("QuantityUnitInput chips", () => {
  it("renders gram chips for the gram unit", () => {
    render(<QuantityUnitInput food={rice} onChange={vi.fn()} />)
    // Default unit = g → chips 50/100/150/200/300.
    for (const v of [50, 100, 150, 200, 300]) {
      expect(screen.getByTestId(`quantity-unit-chip-${v}`)).toBeInTheDocument()
    }
  })

  it("renders count chips (1/2/3) for portion units", () => {
    render(<QuantityUnitInput food={apple} onChange={vi.fn()} />)
    for (const v of [1, 2, 3]) {
      expect(screen.getByTestId(`quantity-unit-chip-${v}`)).toBeInTheDocument()
    }
  })

  it("clicking a chip emits a new amount with the same unit", async () => {
    const onChange = vi.fn()
    render(<QuantityUnitInput food={rice} onChange={onChange} />)
    onChange.mockClear()
    const user = userEvent.setup()
    await user.click(screen.getByTestId("quantity-unit-chip-200"))
    expect(onChange).toHaveBeenCalledWith({ amount: 200, unit: "g" })
  })
})

describe("QuantityUnitInput grams pill confidence", () => {
  it("shows 'exact' (portion) when the food's portion table covers the unit", () => {
    render(<QuantityUnitInput food={apple} onChange={vi.fn()} />)
    const pill = screen.getByTestId("quantity-unit-grams")
    expect(pill).toHaveAttribute("data-source", "portion")
    expect(pill.textContent).toMatch(/180 g/)
  })

  it("shows 'approx' (default) for a universal mass like oz", () => {
    const onChange = vi.fn()
    render(
      <QuantityUnitInput food={rice} recentUnit="oz" onChange={onChange} />
    )
    const pill = screen.getByTestId("quantity-unit-grams")
    expect(pill).toHaveAttribute("data-source", "default")
  })

  it("shows 'estimate' (fallback) for ml on a non-volume food", () => {
    render(<QuantityUnitInput food={rice} recentUnit="ml" onChange={vi.fn()} />)
    const pill = screen.getByTestId("quantity-unit-grams")
    expect(pill).toHaveAttribute("data-source", "fallback")
  })
})

describe("QuantityUnitInput emit guard", () => {
  it("never emits amount = 0 when the field is cleared", async () => {
    const onChange = vi.fn()
    render(<QuantityUnitInput food={rice} onChange={onChange} />)
    onChange.mockClear()
    const user = userEvent.setup()
    const input = screen.getByTestId("quantity-unit-amount") as HTMLInputElement
    await user.clear(input)
    // Clearing must not emit an amount=0 to the parent.
    for (const call of onChange.mock.calls) {
      expect(call[0].amount).not.toBe(0)
    }
  })
})
