import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { mockScaled } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/substitutions", () => ({
  getSubstitutes: vi.fn(),
  applySubstitution: vi.fn(),
}))
vi.mock("@/lib/api/foods", () => ({ revertFoodChange: vi.fn() }))
vi.mock("@/lib/toast", () => ({ toast: vi.fn(), toastError: vi.fn() }))

import { applySubstitution, getSubstitutes } from "@/lib/api/substitutions"

import { SubstituteButton, SubstituteDialog } from "./SubstituteDialog"

const SHALLOT = {
  food_id: 90,
  name: "Shallot",
  ratio: 1,
  amount: { amount: 180, unit: "g" },
  note_en: "",
  note_de: "",
}

describe("SubstituteDialog", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
    vi.mocked(getSubstitutes).mockResolvedValue({ options: [SHALLOT] })
    vi.mocked(applySubstitution).mockResolvedValue({
      undo: { first_event_id: 4, last_event_id: 4 },
    })
  })

  it("cook mode: pick the missing ingredient, then a substitute with its amount", async () => {
    const user = userEvent.setup()
    renderWithRouter(
      <SubstituteButton sessionId={7} ingredients={mockScaled.ingredients} />
    )
    await user.click(await screen.findByTestId("substitute-open"))
    await user.click(screen.getByRole("button", { name: "Onion" }))

    expect(getSubstitutes).toHaveBeenCalledWith(51, [7])
    const option = await screen.findByTestId("substitute-option")
    expect(option).toHaveTextContent("180 g Shallot")

    await user.click(option)
    await waitFor(() =>
      expect(applySubstitution).toHaveBeenCalledWith(7, 51, 90)
    )
    await waitFor(() =>
      expect(screen.queryByTestId("substitute-dialog")).not.toBeInTheDocument()
    )
  })

  it("shop: applies to every batch that needs the item and says so", async () => {
    const user = userEvent.setup()
    renderWithRouter(
      <SubstituteDialog
        open
        onOpenChange={() => {}}
        sessionIds={[7, 8]}
        from={{ id: 51, name: "Onion" }}
      />
    )
    expect(
      await screen.findByText("This applies to all 2 batches that need it.")
    ).toBeInTheDocument()
    await user.click(await screen.findByTestId("substitute-option"))
    await waitFor(() => expect(applySubstitution).toHaveBeenCalledTimes(2))
    expect(applySubstitution).toHaveBeenNthCalledWith(1, 7, 51, 90)
    expect(applySubstitution).toHaveBeenNthCalledWith(2, 8, 51, 90)
  })

  it("says so when nothing can be offered", async () => {
    vi.mocked(getSubstitutes).mockResolvedValue({ options: [] })
    renderWithRouter(
      <SubstituteDialog
        open
        onOpenChange={() => {}}
        sessionIds={[7]}
        from={{ id: 51, name: "Onion" }}
      />
    )
    expect(await screen.findByTestId("substitute-none")).toHaveTextContent(
      "No substitute I can offer for Onion."
    )
  })
})
