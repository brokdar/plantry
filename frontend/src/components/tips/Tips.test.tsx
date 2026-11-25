import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Tip, TipsView } from "@/lib/api/tips"
import i18n from "@/lib/i18n"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/tips", () => ({
  getTips: vi.fn(),
  applyTip: vi.fn(),
  dismissTip: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({
  toast: Object.assign(vi.fn(), { success: vi.fn() }),
  toastError: vi.fn(),
}))

import { applyTip, dismissTip, getTips } from "@/lib/api/tips"

import { FitPhrase, TipMark, TipsProvider } from "./Tips"

const lime: Tip = {
  id: 9,
  plate_id: 5,
  kind: "finishing_acid",
  dimension: "taste_balance",
  state: "shown",
  action: "add",
  food_id: 30,
  food_name: "Lime wedges",
  grams: 10,
}

function view(over: Partial<TipsView> = {}): TipsView {
  return {
    on: true,
    start: "2026-11-23",
    tips: [lime, { ...lime, id: 10, plate_id: 7, state: "dismissed" }],
    fit: [
      { plate_id: 5, raised: ["taste_balance", "texture"] },
      { plate_id: 6, raised: [] },
    ],
    ...over,
  }
}

function renderPlates() {
  renderWithRouter(
    <TipsProvider date="2026-11-25">
      {[5, 6, 7].map((id) => (
        <div key={id} data-testid={`plate-${id}`}>
          <TipMark plateId={id} />
          <FitPhrase plateId={id} />
        </div>
      ))}
    </TipsProvider>
  )
}

describe("tips", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
    vi.mocked(getTips).mockResolvedValue(view())
    vi.mocked(applyTip).mockResolvedValue({
      undo: { token: "t" } as never,
    })
    vi.mocked(dismissTip).mockResolvedValue(undefined)
  })

  it("marks only the plate with a shown tip (TIP-1)", async () => {
    renderPlates()
    expect(
      await within(await screen.findByTestId("plate-5")).findByTestId(
        "tip-mark"
      )
    ).toBeVisible()
    expect(screen.getAllByTestId("tip-mark")).toHaveLength(1)
    expect(getTips).toHaveBeenCalledWith("2026-11-25")
  })

  it("opens the sentence and applies the tip", async () => {
    renderPlates()
    await userEvent.click(
      await screen.findByRole("button", { name: "Tip for this meal" })
    )
    const pop = await screen.findByTestId("tip-popover")
    expect(pop).toHaveTextContent(
      "Finish with Lime wedges. Something sharp lifts the rich plate."
    )
    await userEvent.click(within(pop).getByRole("button", { name: "Apply" }))
    await waitFor(() => expect(applyTip).toHaveBeenCalledWith(9))
    expect(dismissTip).not.toHaveBeenCalled()
  })

  it("dismisses the tip with Not now", async () => {
    renderPlates()
    await userEvent.click(
      await screen.findByRole("button", { name: "Tip for this meal" })
    )
    await userEvent.click(
      within(await screen.findByTestId("tip-popover")).getByRole("button", {
        name: "Not now",
      })
    )
    await waitFor(() => expect(dismissTip).toHaveBeenCalledWith(9))
    expect(applyTip).not.toHaveBeenCalled()
  })

  it("names a swap's two sides", async () => {
    vi.mocked(getTips).mockResolvedValue(
      view({
        tips: [
          {
            ...lime,
            kind: "side_swap",
            dimension: "sauce_starch",
            action: "swap",
            food_name: "Rice",
            from_name: "Green salad",
          },
        ],
      })
    )
    renderPlates()
    await userEvent.click(
      await screen.findByRole("button", { name: "Tip for this meal" })
    )
    expect(await screen.findByTestId("tip-popover")).toHaveTextContent(
      "Swap Green salad for Rice. Something to soak up the sauce."
    )
  })

  it("phrases the plate fit", async () => {
    renderPlates()
    expect(
      await within(await screen.findByTestId("plate-5")).findByTestId(
        "fit-phrase"
      )
    ).toHaveTextContent("This plate could use something sharp, crunch.")
    expect(
      within(screen.getByTestId("plate-6")).getByTestId("fit-phrase")
    ).toHaveTextContent("A well-rounded plate.")
    expect(
      within(screen.getByTestId("plate-7")).queryByTestId("fit-phrase")
    ).toBeNull()
  })

  it("shows nothing while the Chef is off", async () => {
    vi.mocked(getTips).mockResolvedValue({ on: false, tips: [], fit: [] })
    renderPlates()
    await waitFor(() => expect(getTips).toHaveBeenCalled())
    expect(await screen.findByTestId("plate-5")).toBeInTheDocument()
    expect(screen.queryByTestId("tip-mark")).toBeNull()
    expect(screen.queryByTestId("fit-phrase")).toBeNull()
  })
})
