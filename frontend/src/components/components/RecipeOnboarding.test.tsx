import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"
import { screen } from "@testing-library/react"

import { toast } from "@/lib/toast"
import { renderWithRouter } from "@/test/render"
import { RECIPE_GUIDE_KEY, RecipeOnboarding } from "./RecipeOnboarding"

describe("RecipeOnboarding", () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(toast, "success").mockImplementation(() => "id")
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  test("below the target it shows the progress row and both option cards", async () => {
    renderWithRouter(<RecipeOnboarding count={4} target={10} />)

    expect(
      await screen.findByText(
        "4 of 10 recipes. Plantry needs 10 to plan a varied week."
      )
    ).toBeInTheDocument()
    const bar = screen.getByRole("progressbar")
    expect(bar).toHaveAttribute("aria-valuenow", "4")
    expect(bar).toHaveAttribute("aria-valuemax", "10")
    expect(bar).toHaveAttribute("aria-valuetext", "4 of 10 recipes")

    expect(
      screen.getByRole("heading", { name: "Import from a link" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("heading", { name: "Create manually" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("link", { name: "Import a recipe" })
    ).toHaveAttribute("href", "/import")
    expect(
      screen.getByRole("link", { name: "Create a recipe" })
    ).toHaveAttribute("href", "/components/new")
  })

  test("at the target it renders no cards", async () => {
    renderWithRouter(
      <>
        <p>page</p>
        <RecipeOnboarding count={10} target={10} />
      </>
    )
    await screen.findByText("page")
    expect(screen.queryByRole("progressbar")).toBeNull()
    expect(
      screen.queryByRole("heading", { name: "Import from a link" })
    ).toBeNull()
  })

  test("showing the guide sets the flag", async () => {
    renderWithRouter(<RecipeOnboarding count={3} target={10} />)
    await screen.findByRole("progressbar")
    expect(localStorage.getItem(RECIPE_GUIDE_KEY)).toBe("shown")
  })

  test("the milestone toast fires once across re-renders when the flag is set", async () => {
    localStorage.setItem(RECIPE_GUIDE_KEY, "shown")
    const { rerender } = renderWithRouter(
      <RecipeOnboarding count={10} target={10} />
    )
    await vi.waitFor(() => expect(toast.success).toHaveBeenCalledTimes(1))
    expect(toast.success).toHaveBeenCalledWith(
      "10 recipes. You're ready to plan your first week."
    )

    rerender(<RecipeOnboarding count={11} target={10} />)
    rerender(<RecipeOnboarding count={10} target={10} />)
    expect(toast.success).toHaveBeenCalledTimes(1)
    expect(localStorage.getItem(RECIPE_GUIDE_KEY)).toBe("done")
  })

  test("no toast for a library that never showed the guide", async () => {
    renderWithRouter(<RecipeOnboarding count={50} target={10} />)
    await new Promise((r) => setTimeout(r, 0))
    expect(toast.success).not.toHaveBeenCalled()
  })

  test("survives storage that throws", async () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked")
    })
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked")
    })
    renderWithRouter(<RecipeOnboarding count={4} target={10} />)
    expect(await screen.findByRole("progressbar")).toBeInTheDocument()
  })
})
