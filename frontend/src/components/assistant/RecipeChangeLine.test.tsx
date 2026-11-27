import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"

import { RecipeChangeLine } from "./RecipeChangeLine"

vi.mock("@/lib/api/foods", () => ({ revertFoodChange: vi.fn() }))
import { revertFoodChange } from "@/lib/api/foods"

describe("RecipeChangeLine", () => {
  beforeEach(() => {
    vi.mocked(revertFoodChange).mockReset().mockResolvedValue({})
  })

  it("Undo reverts the food change and marks the line undone", async () => {
    const undo = { first_event_id: 3, last_event_id: 4 }
    renderWithRouter(
      <RecipeChangeLine
        change={{ id: "v", tool: "create_variant", recipeId: 9, undo }}
      />
    )
    await userEvent.click(
      await screen.findByTestId("assistant-recipe-change-undo")
    )
    await waitFor(() =>
      expect(screen.getByTestId("assistant-recipe-change")).toHaveAttribute(
        "data-state",
        "undone"
      )
    )
    expect(revertFoodChange).toHaveBeenCalledWith(undo)
  })
})
