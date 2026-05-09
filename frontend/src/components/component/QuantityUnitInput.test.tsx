import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { describe, expect, it, vi } from "vitest"
import "@/lib/i18n"

import type { LeafFood } from "@/lib/api/foods"

import {
  QuantityUnitInput,
  defaultQuantityValueForFood,
  type QuantityUnitValue,
} from "./QuantityUnitInput"

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

/** Controlled host that mirrors the production wiring: parent owns the
 *  (amount, unit) state and seeds the initial value via the exported helper. */
function Host({
  food,
  recentUnit,
  onChange,
}: {
  food: LeafFood
  recentUnit?: string | null
  onChange?: (next: QuantityUnitValue) => void
}) {
  const [value, setValue] = useState<QuantityUnitValue>(() =>
    defaultQuantityValueForFood(food, recentUnit)
  )
  return (
    <QuantityUnitInput
      food={food}
      value={value}
      onChange={(next) => {
        setValue(next)
        onChange?.(next)
      }}
    />
  )
}

describe("defaultQuantityValueForFood", () => {
  it("default unit = first entry from food's portions", () => {
    const v = defaultQuantityValueForFood(apple)
    expect(v.unit).toBe("apple")
    expect(v.amount).toBe(1)
  })

  it("default unit falls back to g when food has no portions", () => {
    const v = defaultQuantityValueForFood(rice)
    expect(v.unit).toBe("g")
    expect(v.amount).toBe(100)
  })

  it("recentUnit overrides the default when set", () => {
    const v = defaultQuantityValueForFood(apple, "g")
    expect(v.unit).toBe("g")
  })
})

describe("QuantityUnitInput chips", () => {
  it("renders gram chips for the gram unit", () => {
    render(<Host food={rice} />)
    for (const v of [50, 100, 150, 200, 300]) {
      expect(screen.getByTestId(`quantity-unit-chip-${v}`)).toBeInTheDocument()
    }
  })

  it("renders count chips (1/2/3) for portion units", () => {
    render(<Host food={apple} />)
    for (const v of [1, 2, 3]) {
      expect(screen.getByTestId(`quantity-unit-chip-${v}`)).toBeInTheDocument()
    }
  })

  it("clicking a chip emits a new amount with the same unit", async () => {
    const onChange = vi.fn()
    render(<Host food={rice} onChange={onChange} />)
    const user = userEvent.setup()
    await user.click(screen.getByTestId("quantity-unit-chip-200"))
    expect(onChange).toHaveBeenCalledWith({ amount: 200, unit: "g" })
  })
})

describe("QuantityUnitInput grams pill confidence", () => {
  it("shows 'exact' (portion) when the food's portion table covers the unit", () => {
    render(<Host food={apple} />)
    const pill = screen.getByTestId("quantity-unit-grams")
    expect(pill).toHaveAttribute("data-source", "portion")
    expect(pill.textContent).toMatch(/180 g/)
  })

  it("shows 'approx' (default) for a universal mass like oz", () => {
    render(<Host food={rice} recentUnit="oz" />)
    const pill = screen.getByTestId("quantity-unit-grams")
    expect(pill).toHaveAttribute("data-source", "default")
  })

  it("shows 'estimate' (fallback) for ml on a non-volume food", () => {
    render(<Host food={rice} recentUnit="ml" />)
    const pill = screen.getByTestId("quantity-unit-grams")
    expect(pill).toHaveAttribute("data-source", "fallback")
  })
})

describe("QuantityUnitInput emit guard", () => {
  it("never emits amount = 0 when the field is cleared", async () => {
    const onChange = vi.fn()
    render(<Host food={rice} onChange={onChange} />)
    const user = userEvent.setup()
    const input = screen.getByTestId("quantity-unit-amount") as HTMLInputElement
    await user.clear(input)
    for (const call of onChange.mock.calls) {
      expect(call[0].amount).not.toBe(0)
    }
  })
})
