import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { mockSettingsList } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/settings", () => ({
  listSettings: vi.fn(),
  setSetting: vi.fn(),
  clearSetting: vi.fn(),
  listAIModels: vi.fn(),
  getSystemInfo: vi.fn(),
}))

import { listSettings, setSetting } from "@/lib/api/settings"

import { CookingSection } from "./CookingSection"

/** A settings "server": GET returns what PUT stored. */
function withValues(values: Record<string, string> = {}) {
  const stored = { ...values }
  vi.mocked(listSettings).mockImplementation(async () =>
    mockSettingsList(
      Object.entries(stored).map(([key, value]) => ({
        key,
        value,
        source: "db" as const,
      }))
    )
  )
  vi.mocked(setSetting).mockImplementation(async (key, value) => {
    stored[key] = value
  })
}

function renderSection() {
  return renderWithRouter(
    <TooltipProvider>
      <CookingSection />
    </TooltipProvider>
  )
}

/** The setting row whose label is `label`. */
function row(label: string) {
  const el = screen.getByText(label, { selector: "p, label" })
  return el.closest(".grid") as HTMLElement
}

describe("CookingSection", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    withValues()
  })

  it("renders every field with its default and effect line", async () => {
    renderSection()
    await screen.findByText("Cooking sessions per day")
    expect(
      screen.getByText("Plantry plans at most 1 cooking session per day.")
    ).toBeInTheDocument()
    expect(screen.getByLabelText("Batch cooking")).toBeChecked()
    expect(
      screen.getByText("Plantry cooks larger batches and plans the leftovers.")
    ).toBeInTheDocument()
    expect(
      screen.getByText("Plantry plans leftovers up to 3 days after cooking.")
    ).toBeInTheDocument()
    expect(within(row("Leftovers keep for")).getByText("3 days")).toBeVisible()
    expect(screen.getByLabelText("Freezing")).not.toBeChecked()
    expect(screen.queryByText("Frozen portions keep for")).toBeNull()
    expect(
      screen.getByText("Plantry doesn't repeat a recipe within 3 weeks.")
    ).toBeInTheDocument()
    expect(
      screen.getByText("Plantry plans only from your recipes.")
    ).toBeInTheDocument()
    expect(screen.getByText("You shop on Monday.")).toBeInTheDocument()
    expect(
      screen.getByTestId("plan-anchor-radio-next_shopping_day")
    ).toBeChecked()
    expect(screen.queryByTestId("keep-days-warning")).toBeNull()
  })

  it("changing keep days to 5 saves it and shows the food-safety warning", async () => {
    withValues({ "plan.default_keep_days": "4" })
    const user = userEvent.setup()
    renderSection()
    await user.click(
      await screen.findByRole("button", { name: "Increase Leftovers keep for" })
    )
    await waitFor(() =>
      expect(setSetting).toHaveBeenCalledWith("plan.default_keep_days", "5")
    )
    expect(await screen.findByTestId("keep-days-warning")).toHaveTextContent(
      "Cooked food usually keeps 3 to 4 days in the fridge."
    )
  })

  it("stepping cooking sessions saves the new count and shows Saved", async () => {
    const user = userEvent.setup()
    renderSection()
    await user.click(
      await screen.findByRole("button", {
        name: "Increase Cooking sessions per day",
      })
    )
    await waitFor(() =>
      expect(setSetting).toHaveBeenCalledWith(
        "plan.max_cook_occasions_per_day",
        "2"
      )
    )
    expect(
      await within(row("Cooking sessions per day")).findByText("Saved")
    ).toBeInTheDocument()
  })

  it("turning freezing on reveals freezer keep days, saved on blur", async () => {
    withValues({ "plan.freezing_enabled": "true" })
    const user = userEvent.setup()
    renderSection()
    const input = await screen.findByTestId("freezer-keep-input")
    expect(input).toHaveValue(90)
    await user.clear(input)
    await user.type(input, "60")
    expect(setSetting).not.toHaveBeenCalled()
    await user.tab()
    await waitFor(() =>
      expect(setSetting).toHaveBeenCalledWith("plan.freezer_keep_days", "60")
    )
  })

  it("rejects a freezer keep value out of range without saving", async () => {
    withValues({ "plan.freezing_enabled": "true" })
    const user = userEvent.setup()
    renderSection()
    const input = await screen.findByTestId("freezer-keep-input")
    await user.clear(input)
    await user.type(input, "400")
    await user.tab()
    expect(
      screen.getByText("Enter a whole number from 1 to 365.")
    ).toBeInTheDocument()
    expect(setSetting).not.toHaveBeenCalled()
  })

  it("toggles batch cooking", async () => {
    const user = userEvent.setup()
    renderSection()
    await user.click(await screen.findByLabelText("Batch cooking"))
    await waitFor(() =>
      expect(setSetting).toHaveBeenCalledWith("plan.batch_cooking", "false")
    )
  })

  describe("plan anchor and shopping day", () => {
    it("saves plan.anchor when the anchor radio changes", async () => {
      const user = userEvent.setup()
      renderSection()
      await user.click(await screen.findByTestId("plan-anchor-radio-today"))
      await waitFor(() =>
        expect(setSetting).toHaveBeenCalledWith("plan.anchor", "today")
      )
    })

    it("saves plan.shopping_day when the shopping day changes", async () => {
      const user = userEvent.setup()
      renderSection()
      await user.click(await screen.findByTestId("plan-shopping-day-select"))
      await user.click(
        await screen.findByRole("option", { name: /wednesday/i })
      )
      await waitFor(() =>
        expect(setSetting).toHaveBeenCalledWith("plan.shopping_day", "2")
      )
    })

    it("offers exactly the two anchors", async () => {
      renderSection()
      await screen.findByTestId("plan-anchor-radio-today")
      expect(
        screen.getAllByRole("radio").map((r) => r.getAttribute("value"))
      ).toEqual(["today", "next_shopping_day"])
      expect(screen.queryByTestId("plan-week-starts-on-select")).toBeNull()
    })
  })
})
