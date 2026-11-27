import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Find } from "@/lib/api/finds"
import i18n from "@/lib/i18n"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/finds", () => ({
  getFind: vi.fn(),
  discardFind: vi.fn(),
}))
vi.mock("@/lib/api/kitchen", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/kitchen")>()),
  getKitchen: vi.fn(),
  updateKitchen: vi.fn(),
}))
const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", async (orig) => ({
  ...(await orig<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigateMock,
}))

import { discardFind, getFind } from "@/lib/api/finds"
import { getKitchen, updateKitchen } from "@/lib/api/kitchen"

import { FindResult } from "./FindResult"

function find(over: Partial<Find>): Find {
  return {
    id: 7,
    state: "ready",
    dish: "chicken curry",
    draft: { name: "Mild tofu curry" } as Find["draft"],
    source_url: "https://www.bbcgoodfood.com/recipes/chicken-curry",
    sources: [
      {
        url: "https://www.bbcgoodfood.com/recipes/chicken-curry",
        site: "bbcgoodfood.com",
        rating: 4.8,
        votes: 2310,
        base: true,
      },
    ],
    flags: [{ code: "outlier_oven_temp", value: 250, typical: 200 }],
    origin: "inspired",
    changes: [
      {
        kind: "protein_swap",
        from: "chicken",
        to: "tofu",
        reason: "vegetarian",
      },
    ],
    steps: [{ key: "searched", params: { sites: "bbcgoodfood.com" } }],
    questions: ["equipment:wok"],
    food_id: null,
    ...over,
  }
}

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(getKitchen).mockResolvedValue({
    minutes_weekday: 30,
    minutes_weekend: 60,
    skill: null,
    equipment: ["oven"],
    heat: null,
    leanings: [],
    basics: [],
  })
})

describe("FindResult", () => {
  it("shows the steps in words while searching", async () => {
    vi.mocked(getFind).mockResolvedValue(find({ state: "searching" }))
    renderWithRouter(<FindResult id={7} />)
    expect(await screen.findByText("Searched bbcgoodfood.com")).toBeVisible()
    expect(screen.getByRole("status")).toHaveTextContent("Looking for a recipe")
  })

  it("reviews a ready find: source line, origin, changes, flags", async () => {
    vi.mocked(getFind).mockResolvedValue(find({}))
    const user = userEvent.setup()
    renderWithRouter(<FindResult id={7} />)
    expect(await screen.findByTestId("find-source")).toHaveTextContent(
      "bbcgoodfood.com · rated 4.8 by 2310 cooks"
    )
    expect(screen.getByTestId("find-origin")).toHaveTextContent(
      "Inspired by bbcgoodfood.com"
    )
    expect(screen.getByText("chicken → tofu · vegetarian")).toBeVisible()
    expect(screen.getByText(/Oven temperature \(250\)/)).toBeVisible()

    await user.click(screen.getByTestId("find-add"))
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/recipes",
      search: { add: "find", find: 7 },
    })
  })

  it("Not for me sends the tapped reason", async () => {
    vi.mocked(getFind).mockResolvedValue(find({}))
    vi.mocked(discardFind).mockResolvedValue(find({ state: "discarded" }))
    const user = userEvent.setup()
    renderWithRouter(<FindResult id={7} />)
    await user.click(await screen.findByRole("button", { name: "Not for me" }))
    await user.click(screen.getByRole("button", { name: "Looks wrong" }))
    await waitFor(() =>
      expect(discardFind).toHaveBeenCalledWith(7, "looks_wrong")
    )
  })

  it("a yes to the tool question saves it in the kitchen", async () => {
    vi.mocked(getFind).mockResolvedValue(find({ state: "needs_check" }))
    vi.mocked(updateKitchen).mockResolvedValue({} as never)
    const user = userEvent.setup()
    renderWithRouter(<FindResult id={7} />)
    const q = await screen.findByTestId("find-question")
    expect(q).toHaveTextContent("Do you have this tool: Wok?")
    await user.click(screen.getByRole("button", { name: "Yes" }))
    await waitFor(() =>
      expect(updateKitchen).toHaveBeenCalledWith(
        expect.objectContaining({ equipment: ["oven", "wok"] })
      )
    )
    expect(screen.queryByTestId("find-question")).toBeNull()
  })

  it("a failed find says so", async () => {
    vi.mocked(getFind).mockResolvedValue(
      find({ state: "failed", steps: [{ key: "nothing_found" }] })
    )
    renderWithRouter(<FindResult id={7} />)
    expect(await screen.findByText("No recipe found this time.")).toBeVisible()
    expect(screen.getByText("Found no usable recipe")).toBeVisible()
  })
})
