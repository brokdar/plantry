import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import "@/lib/i18n"
import { NUTRITION_PRESETS } from "@/lib/nutrition-presets"

import {
  AllergiesInput,
  DietTypePicker,
  NutritionGoalFields,
} from "./ProfileEditor"

const noGoal = {
  kcal_target: null,
  protein_pct: null,
  fat_pct: null,
  carbs_pct: null,
}

describe("NutritionGoalFields", () => {
  it("marks the preset the stored targets match", () => {
    render(
      <NutritionGoalFields
        value={NUTRITION_PRESETS.maintain}
        onCommit={vi.fn()}
      />
    )
    expect(screen.getByRole("radio", { name: /^Maintain/ })).toBeChecked()
    expect(screen.getByRole("radio", { name: /^Cut/ })).not.toBeChecked()
    expect(screen.queryByLabelText(/calorie target/i)).toBeNull()
  })

  it("shows the bulk card at 2700 kcal and commits it", async () => {
    const onCommit = vi.fn()
    render(<NutritionGoalFields value={noGoal} onCommit={onCommit} />)
    const bulk = screen.getByRole("radio", { name: /^Bulk/ })
    expect(bulk.closest("label")).toHaveTextContent("2700 kcal")
    // The split names its macros, visibly and for screen readers.
    expect(bulk.closest("label")).toHaveTextContent("P 25 / F 25 / C 50 %")
    expect(bulk).toHaveAccessibleName(
      expect.stringContaining("25 % protein, 25 % fat, 50 % carbs")
    )
    await userEvent.click(bulk)
    expect(onCommit).toHaveBeenCalledWith(NUTRITION_PRESETS.bulk)
  })

  it("opens custom fields prefilled with targets that match no preset", () => {
    render(
      <NutritionGoalFields
        value={{
          kcal_target: 2000,
          protein_pct: 30,
          fat_pct: 30,
          carbs_pct: 40,
        }}
        onCommit={vi.fn()}
      />
    )
    expect(screen.getByRole("radio", { name: /^Custom/ })).toBeChecked()
    expect(screen.getByLabelText(/calorie target/i)).toHaveValue(2000)
    expect(screen.getByLabelText(/protein %/i)).toHaveValue(30)
  })

  it("editing one custom field updates only that field", async () => {
    const onCommit = vi.fn()
    const user = userEvent.setup()
    render(
      <NutritionGoalFields
        value={{
          kcal_target: 2000,
          protein_pct: 30,
          fat_pct: 30,
          carbs_pct: 40,
        }}
        onCommit={onCommit}
      />
    )
    const protein = screen.getByLabelText(/protein %/i)
    await user.clear(protein)
    await user.type(protein, "25")
    await user.click(document.body)
    await waitFor(() =>
      expect(onCommit).toHaveBeenCalledWith({
        kcal_target: 2000,
        protein_pct: 25,
        fat_pct: 30,
        carbs_pct: 40,
      })
    )
  })
})

describe("DietTypePicker", () => {
  it("offers the four diet types", async () => {
    const onChange = vi.fn()
    render(<DietTypePicker value="omnivore" onChange={onChange} />)
    for (const name of ["Vegan", "Vegetarian", "Pescetarian", "Omnivore"]) {
      expect(screen.getByRole("radio", { name })).toBeInTheDocument()
    }
    await userEvent.click(screen.getByRole("radio", { name: "Pescetarian" }))
    expect(onChange).toHaveBeenCalledWith("pescetarian")
  })
})

describe("AllergiesInput", () => {
  it("adds and removes chips", async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    const { rerender } = render(
      <AllergiesInput value={[]} onChange={onChange} />
    )
    await user.type(
      screen.getByLabelText("Allergies and restrictions"),
      "gluten{Enter}"
    )
    expect(onChange).toHaveBeenLastCalledWith(["gluten"])

    rerender(<AllergiesInput value={["gluten"]} onChange={onChange} />)
    await user.click(screen.getByRole("button", { name: "Remove gluten" }))
    expect(onChange).toHaveBeenLastCalledWith([])
  })
})
