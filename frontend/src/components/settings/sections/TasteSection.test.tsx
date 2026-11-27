import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/taste", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/taste")>()),
  getTaste: vi.fn(),
  setItemPolarity: vi.fn(),
  removeItem: vi.fn(),
  makeItemNever: vi.fn(),
  removeNever: vi.fn(),
}))
vi.mock("@/lib/api/foods", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/foods")>()),
  revertFoodChange: vi.fn(),
}))

import { revertFoodChange } from "@/lib/api/foods"
import {
  getTaste,
  makeItemNever,
  removeItem,
  setItemPolarity,
  type Taste,
  type TasteItem,
} from "@/lib/api/taste"

import { TasteSection } from "./TasteSection"

const undo = { first_event_id: 7, last_event_id: 7 }
const item = (over: Partial<TasteItem>): TasteItem => ({
  id: 1,
  level: "hero",
  key: "coriander",
  scope: "always",
  source: "statement",
  polarity: "rather_not",
  score: -1,
  ...over,
})
const taste = (over: Partial<Taste> = {}): Taste => ({
  items: [],
  recent: [],
  never: [],
  staples: [],
  ...over,
})

describe("TasteSection", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(setItemPolarity).mockResolvedValue({ undo })
    vi.mocked(removeItem).mockResolvedValue({ undo })
    vi.mocked(makeItemNever).mockResolvedValue(undefined)
    vi.mocked(revertFoodChange).mockResolvedValue({})
  })

  it("lists every group in plain sentences, each item with its source", async () => {
    vi.mocked(getTaste).mockResolvedValue(
      taste({
        never: [{ id: 5, name: "Peanuts" }],
        staples: [{ id: 9, name: "Lasagne" }],
        items: [
          item({ id: 1 }),
          item({
            id: 2,
            key: "thai",
            level: "cuisine",
            source: "learned",
            polarity: "love",
            score: 0.4,
          }),
          item({
            id: 3,
            key: "heat",
            level: "flavour",
            source: "learned",
            polarity: "love",
            score: 0.3,
          }),
          item({
            id: 4,
            key: "fish",
            source: "user",
            polarity: "love",
            score: 1,
          }),
        ],
      })
    )
    renderWithRouter(<TasteSection />)
    expect(await screen.findByText("You never eat Peanuts.")).toBeVisible()
    const rather = screen.getByTestId("taste-rather_not")
    expect(
      within(rather).getByText("You'd rather not have coriander.")
    ).toBeVisible()
    expect(within(rather).getByTestId("taste-source")).toHaveTextContent(
      "You said so"
    )
    const love = screen.getByTestId("taste-love")
    expect(within(love).getByText("You love fish.")).toBeVisible()
    expect(within(love).getByTestId("taste-source")).toHaveTextContent(
      "You set this"
    )
    const leanings = screen.getByTestId("taste-leanings")
    expect(
      within(leanings).getByText("You seem to like spicy dishes.")
    ).toBeVisible()
    expect(within(leanings).getByTestId("taste-source")).toHaveTextContent(
      "Noticed from your meals"
    )
    expect(
      within(screen.getByTestId("taste-cuisines")).getByText("You love Thai.")
    ).toBeVisible()
    expect(
      within(screen.getByTestId("taste-staples")).getByText("Lasagne")
    ).toBeVisible()
  })

  it("removes, flips and makes an item never", async () => {
    vi.mocked(getTaste).mockResolvedValue(taste({ items: [item({ id: 1 })] }))
    renderWithRouter(<TasteSection />)
    await screen.findByText("You'd rather not have coriander.")
    await userEvent.click(
      screen.getByRole("button", { name: "Make it a love" })
    )
    await waitFor(() => expect(setItemPolarity).toHaveBeenCalledWith(1, "love"))
    await userEvent.click(screen.getByRole("button", { name: "Never" }))
    await waitFor(() => expect(makeItemNever).toHaveBeenCalledWith(1))
    await userEvent.click(
      screen.getByRole("button", { name: "Remove coriander" })
    )
    await waitFor(() => expect(removeItem).toHaveBeenCalledWith(1))
  })

  it("Undo of a recently learned item reverts its event", async () => {
    vi.mocked(getTaste).mockResolvedValue(
      taste({
        recent: [
          {
            item: item({
              id: 2,
              key: "thai",
              level: "cuisine",
              source: "learned",
              polarity: "love",
              score: 0.3,
            }),
            at: "2026-03-01T10:00:00Z",
            undo,
          },
        ],
      })
    )
    renderWithRouter(<TasteSection />)
    const recent = await screen.findByTestId("taste-recent")
    await userEvent.click(within(recent).getByRole("button", { name: "Undo" }))
    await waitFor(() => expect(revertFoodChange).toHaveBeenCalledWith(undo))
  })

  it("says so when there is nothing yet", async () => {
    vi.mocked(getTaste).mockResolvedValue(taste())
    renderWithRouter(<TasteSection />)
    expect(
      await screen.findByText("Nothing is on the never list.")
    ).toBeVisible()
    expect(screen.getByText("No staples yet.", { exact: false })).toBeVisible()
  })
})
