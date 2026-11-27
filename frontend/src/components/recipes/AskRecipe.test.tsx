import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import type { RecipeCheck } from "@/lib/api/recipechange"
import { renderWithRouter } from "@/test/render"

import { AskRecipe } from "./AskRecipe"

vi.mock("@/lib/api/recipechange", () => ({
  askAboutRecipe: vi.fn(),
  saveRecipeEdit: vi.fn(),
  checkRecipe: vi.fn(),
  getRecipeChanges: vi.fn(),
}))
vi.mock("@/lib/api/foods", () => ({ revertFoodChange: vi.fn() }))

import {
  askAboutRecipe,
  checkRecipe,
  getRecipeChanges,
  saveRecipeEdit,
} from "@/lib/api/recipechange"

const okCheck: RecipeCheck = { lines: [], blocks: [], flags: [] }
const undo = { first_event_id: 1, last_event_id: 1 }

describe("AskRecipe", () => {
  beforeEach(() => {
    vi.mocked(askAboutRecipe).mockReset()
    vi.mocked(saveRecipeEdit).mockReset()
    vi.mocked(checkRecipe).mockReset()
    vi.mocked(getRecipeChanges)
      .mockReset()
      .mockResolvedValue({ items: ["Added chili."] })
  })

  it("lists the changes list of the recipe", async () => {
    renderWithRouter(<AskRecipe recipeId={7} />)
    expect(await screen.findByText("Added chili.")).toBeInTheDocument()
  })

  it("Make a variant sends the request and links the saved variant", async () => {
    vi.mocked(askAboutRecipe).mockResolvedValue({
      kind: "variant",
      proposal: { name: "Spicy", ingredients: [], steps: [], changes: [] },
      variant: { food_id: 9, changes: ["More heat."], undo, check: okCheck },
    })
    renderWithRouter(<AskRecipe recipeId={7} />)
    await userEvent.click(await screen.findByTestId("ask-chip-variant"))
    expect(await screen.findByTestId("recipe-chat-variant")).toHaveTextContent(
      "More heat."
    )
    expect(askAboutRecipe).toHaveBeenCalledWith(7, expect.any(String))
    expect(screen.getByRole("link", { name: "Spicy" })).toBeInTheDocument()
  })

  it("a blocked variant says why and saves nothing", async () => {
    vi.mocked(askAboutRecipe).mockResolvedValue({
      kind: "variant",
      variant: {
        food_id: 0,
        changes: [],
        undo,
        check: {
          lines: [],
          flags: [],
          blocks: [
            {
              code: "allergen",
              ingredient: 0,
              step: -1,
              params: { name: "Walnuts", allergen: "nuts" },
              reason: "Walnuts contains nuts.",
            },
          ],
        },
      },
    })
    renderWithRouter(<AskRecipe recipeId={7} />)
    await userEvent.type(
      await screen.findByTestId("ask-input"),
      "add walnuts{Enter}"
    )
    expect(await screen.findByTestId("recipe-check-block")).toHaveTextContent(
      "Walnuts contains nuts."
    )
    expect(screen.queryByTestId("recipe-chat-variant")).toBeNull()
  })

  it("an edit shows the diff and the confirmed-plates warning, then saves on tap", async () => {
    const proposal = {
      name: "Curry",
      ingredients: ["a"],
      steps: [],
      changes: [],
    }
    vi.mocked(askAboutRecipe).mockResolvedValue({
      kind: "edit",
      proposal,
      preview: {
        before: { name: "Curry", ingredients: ["400 g rice"], steps: [] },
        after: { name: "Curry", ingredients: ["300 g rice"], steps: [] },
        confirmed_dates: ["2026-12-01"],
        check: okCheck,
      },
    })
    vi.mocked(saveRecipeEdit).mockResolvedValue({
      food_id: 7,
      changes: [],
      undo,
      check: okCheck,
    })
    renderWithRouter(<AskRecipe recipeId={7} />)
    await userEvent.type(
      await screen.findByTestId("ask-input"),
      "use less rice{Enter}"
    )
    expect(await screen.findByTestId("diff-removed")).toHaveTextContent(
      "400 g rice"
    )
    expect(screen.getByTestId("diff-added")).toHaveTextContent("300 g rice")
    expect(screen.getByTestId("recipe-edit-warning")).toHaveTextContent(
      "2026-12-01"
    )
    await userEvent.click(screen.getByTestId("recipe-edit-save"))
    await waitFor(() =>
      expect(saveRecipeEdit).toHaveBeenCalledWith(7, proposal)
    )
    await waitFor(() =>
      expect(screen.queryByTestId("recipe-edit-preview")).toBeNull()
    )
  })

  it("Check this recipe runs the check without asking the model", async () => {
    vi.mocked(checkRecipe).mockResolvedValue({
      lines: [{ text: "1 pinch magic", matched: false }],
      blocks: [],
      flags: [],
    })
    renderWithRouter(<AskRecipe recipeId={7} />)
    await userEvent.click(await screen.findByTestId("ask-chip-check"))
    expect(await screen.findByTestId("recipe-check")).toHaveTextContent(
      "1 pinch magic"
    )
    expect(askAboutRecipe).not.toHaveBeenCalled()
  })
})
