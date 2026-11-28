import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { LibraryReview as Card } from "@/lib/api/library-review"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/library-review", () => ({
  getLibraryReview: vi.fn(),
  dismissLibraryReview: vi.fn(),
  makeReviewVariants: vi.fn(),
}))
vi.mock("@/lib/api/foods", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/foods")>()),
  archiveFood: vi.fn(),
  restoreFood: vi.fn(),
}))

import { archiveFood } from "@/lib/api/foods"
import {
  dismissLibraryReview,
  getLibraryReview,
  makeReviewVariants,
} from "@/lib/api/library-review"

import { LibraryReview } from "./LibraryReview"

const card: Card = {
  month: "2026-11",
  findings: [
    {
      key: "dup:1-2",
      kind: "duplicates",
      recipes: [
        { id: 1, name: "Chili" },
        { id: 2, name: "Chili con Carne" },
      ],
    },
    {
      key: "replaced:3",
      kind: "replaced",
      count: 4,
      recipes: [{ id: 3, name: "Pizza" }],
    },
    {
      key: "data",
      kind: "data",
      count: 2,
      recipes: [
        { id: 4, name: "Soup", missing: ["times", "steps"] },
        { id: 5, name: "Stew", missing: ["times"] },
      ],
    },
  ],
}

describe("LibraryReview", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getLibraryReview).mockResolvedValue({ card })
    vi.mocked(dismissLibraryReview).mockResolvedValue(undefined)
    vi.mocked(makeReviewVariants).mockResolvedValue(undefined)
    vi.mocked(archiveFood).mockResolvedValue({} as never)
  })

  it("asks nothing while the review is off", () => {
    renderWithRouter(<LibraryReview enabled={false} />)
    expect(screen.queryByTestId("library-review")).toBeNull()
    expect(getLibraryReview).not.toHaveBeenCalled()
  })

  it("is absent without a card", async () => {
    vi.mocked(getLibraryReview).mockResolvedValue({ card: null })
    renderWithRouter(<LibraryReview enabled />)
    await waitFor(() => expect(getLibraryReview).toHaveBeenCalled())
    expect(screen.queryByTestId("library-review")).toBeNull()
  })

  it("shows the three finding kinds with their actions", async () => {
    renderWithRouter(<LibraryReview enabled />)
    const dup = await screen.findByTestId("review-finding-duplicates")
    expect(dup).toHaveTextContent("Chili and Chili con Carne look a lot alike.")
    expect(screen.getByTestId("review-finding-replaced")).toHaveTextContent(
      "Pizza was swapped out of plans 4 times lately."
    )
    const data = screen.getByTestId("review-finding-data")
    expect(data).toHaveTextContent("2 recipes are missing details:")
    expect(data).toHaveTextContent("Soup: no times, no steps")
    expect(screen.getByRole("link", { name: "Stew" })).toHaveAttribute(
      "href",
      "/recipes/5/edit"
    )
  })

  it("makes variants, hides a recipe from planning and dismisses", async () => {
    const user = userEvent.setup()
    renderWithRouter(<LibraryReview enabled />)
    await screen.findByTestId("library-review")

    await user.click(screen.getByRole("button", { name: "Make variants" }))
    await waitFor(() =>
      expect(makeReviewVariants).toHaveBeenCalledWith("dup:1-2")
    )

    await user.click(screen.getByRole("button", { name: "Hide from planning" }))
    await waitFor(() => expect(archiveFood).toHaveBeenCalledWith(3))
    await waitFor(() =>
      expect(dismissLibraryReview).toHaveBeenCalledWith("replaced:3")
    )

    await user.click(screen.getByTestId("review-finding-dismiss-data"))
    await waitFor(() =>
      expect(dismissLibraryReview).toHaveBeenCalledWith("data")
    )

    await user.click(screen.getByTestId("library-review-dismiss"))
    await waitFor(() =>
      expect(dismissLibraryReview).toHaveBeenLastCalledWith(undefined)
    )
  })
})
