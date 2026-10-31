import { act, fireEvent, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { mockScaled } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

import { CookMode, type CookModeProps } from "./CookMode"

const STEPS = [
  "Chop the onions.",
  "Fry the onions 5 min.",
  "Add the beans and simmer 20 Minuten.",
  "Season to taste.",
  "Serve.",
]

function renderCook(props: Partial<CookModeProps> = {}) {
  const onDone = vi.fn()
  const onPortionsChange = vi.fn()
  renderWithRouter(
    <CookMode
      name="Chili"
      steps={STEPS}
      ingredients={mockScaled.ingredients}
      portions={6}
      onPortionsChange={onPortionsChange}
      close={<span />}
      onDone={onDone}
      {...props}
    />
  )
  return { onDone, onPortionsChange }
}

describe("CookMode", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("en")
  })

  it("shows the name, Cooking × 6 and the scaled ingredients", async () => {
    renderCook({ coverage: "Covers Sun dinner, Mon lunch, Mon dinner" })
    expect(
      await screen.findByRole("heading", { level: 1, name: "Chili" })
    ).toBeVisible()
    expect(screen.getByTestId("cook-badge")).toHaveTextContent("Cooking × 6")
    expect(screen.getByTestId("cook-coverage")).toHaveTextContent(
      "Covers Sun dinner, Mon lunch, Mon dinner"
    )
    const rows = screen.getAllByTestId("cook-ingredient")
    expect(rows[0]).toHaveTextContent("Kidney beans")
    expect(rows[0]).toHaveTextContent("600 g")
    expect(rows[1]).toHaveTextContent("3 pcs")
  })

  it("has no coverage text without one (recipe mode)", async () => {
    renderCook()
    await screen.findByTestId("cook-badge")
    expect(screen.queryByTestId("cook-coverage")).toBeNull()
  })

  it("the stepper reports the next portions", async () => {
    const { onPortionsChange } = renderCook()
    await userEvent.click(
      await screen.findByRole("button", { name: /increase portions/i })
    )
    expect(onPortionsChange).toHaveBeenCalledWith(7)
  })

  it("ticks an ingredient off and back", async () => {
    renderCook()
    const box = within(
      (await screen.findAllByTestId("cook-ingredient"))[0]
    ).getByRole("checkbox")
    await userEvent.click(box)
    expect(box).toBeChecked()
    await userEvent.click(box)
    expect(box).not.toBeChecked()
  })

  it("says what to buy for a count unit used in part", async () => {
    renderCook({
      ingredients: [
        {
          ...mockScaled.ingredients[1],
          name: "Tomatoes",
          unit: "piece",
          amount: 0.67,
          display: { amount: 2 / 3, unit: "piece" },
          buy: { amount: 1, unit: "piece" },
        },
      ],
    })
    const row = await screen.findByTestId("cook-ingredient")
    expect(row).toHaveTextContent("⅔ pcs")
    expect(row).toHaveTextContent("1 pcs, using ⅔")
  })

  // F6-AC3
  it("pages through the steps with buttons and arrow keys", async () => {
    renderCook()
    await userEvent.click(await screen.findByRole("tab", { name: "Steps" }))
    const position = screen.getByTestId("step-position")
    expect(position).toHaveTextContent("Step 1 of 5")
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled()

    await userEvent.click(screen.getByRole("button", { name: "Next" }))
    expect(position).toHaveTextContent("Step 2 of 5")
    expect(screen.getByTestId("step-text")).toHaveTextContent(
      "Fry the onions 5 min."
    )

    fireEvent.keyDown(window, { key: "ArrowRight" })
    expect(position).toHaveTextContent("Step 3 of 5")
    fireEvent.keyDown(window, { key: "ArrowLeft" })
    fireEvent.keyDown(window, { key: "ArrowLeft" })
    fireEvent.keyDown(window, { key: "ArrowLeft" })
    expect(position).toHaveTextContent("Step 1 of 5")

    await userEvent.click(screen.getByRole("button", { name: "Previous" }))
    expect(position).toHaveTextContent("Step 1 of 5")
  })

  it("turns the page on a horizontal swipe, not a vertical one", async () => {
    renderCook()
    const text = await screen.findByTestId("step-text")
    const swipe = (dx: number, dy: number) => {
      fireEvent.touchStart(text, { touches: [{ clientX: 200, clientY: 200 }] })
      fireEvent.touchEnd(text, {
        changedTouches: [{ clientX: 200 + dx, clientY: 200 + dy }],
      })
    }
    swipe(-120, 10)
    expect(screen.getByTestId("step-position")).toHaveTextContent("Step 2 of 5")
    swipe(-20, 200)
    expect(screen.getByTestId("step-position")).toHaveTextContent("Step 2 of 5")
    swipe(120, 0)
    expect(screen.getByTestId("step-position")).toHaveTextContent("Step 1 of 5")
  })

  it("on the last step: the freeze reminder and Done cooking", async () => {
    const { onDone } = renderCook({ freezePortions: 2 })
    await screen.findByTestId("step-pager")
    expect(screen.queryByTestId("cook-freeze")).toBeNull()
    expect(screen.queryByTestId("cook-done")).toBeNull()

    for (let i = 0; i < 4; i++) fireEvent.keyDown(window, { key: "ArrowRight" })
    expect(screen.getByTestId("step-position")).toHaveTextContent("Step 5 of 5")
    expect(screen.getByTestId("cook-freeze")).toHaveTextContent(
      "Freeze 2 portions"
    )
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled()
    await userEvent.click(screen.getByTestId("cook-done"))
    expect(onDone).toHaveBeenCalledTimes(1)
  })

  it("has no freeze reminder without frozen portions", async () => {
    renderCook({ steps: ["Serve."] })
    expect(await screen.findByTestId("cook-done")).toBeVisible()
    expect(screen.queryByTestId("cook-freeze")).toBeNull()
  })

  it("offers Done cooking for a recipe without steps", async () => {
    renderCook({ steps: [] })
    expect(
      await screen.findByText("This recipe has no steps yet.")
    ).toBeVisible()
    expect(screen.getByTestId("cook-done")).toBeVisible()
  })

  it("speaks German", async () => {
    await i18n.changeLanguage("de")
    renderCook({ freezePortions: 1, steps: ["Servieren."] })
    expect(await screen.findByTestId("cook-badge")).toHaveTextContent(
      "Kochen × 6"
    )
    expect(screen.getByTestId("step-position")).toHaveTextContent(
      "Schritt 1 von 1"
    )
    expect(screen.getByTestId("cook-freeze")).toHaveTextContent(
      "1 Portion einfrieren"
    )
    expect(screen.getByTestId("cook-done")).toHaveTextContent("Fertig gekocht")
  })

  // F6-AC6
  it("renders without navigator.wakeLock", async () => {
    expect("wakeLock" in navigator).toBe(false)
    renderCook()
    expect(await screen.findByTestId("cook-badge")).toBeVisible()
  })

  describe("timers", () => {
    beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }))
    afterEach(() => vi.useRealTimers())

    it("a duration starts a pill that stays across steps and rings at zero", async () => {
      const vibrate = vi.fn()
      Object.defineProperty(navigator, "vibrate", {
        value: vibrate,
        configurable: true,
      })
      try {
        renderCook()
        await screen.findByTestId("step-pager")
        fireEvent.keyDown(window, { key: "ArrowRight" })
        fireEvent.click(
          screen.getByRole("button", { name: "Start a 5 min timer" })
        )

        const pill = screen.getByTestId("cook-timer")
        expect(pill).toHaveTextContent("5 min")
        expect(pill).toHaveTextContent("5:00")

        fireEvent.keyDown(window, { key: "ArrowRight" })
        await act(() => vi.advanceTimersByTimeAsync(60_000))
        expect(screen.getByTestId("cook-timer")).toHaveTextContent("4:00")

        await act(() => vi.advanceTimersByTimeAsync(4 * 60_000))
        expect(screen.getByTestId("cook-timer")).toHaveTextContent("Done")
        expect(screen.getByTestId("cook-timer")).toHaveAttribute("data-done")
        expect(vibrate).toHaveBeenCalledTimes(1)

        fireEvent.click(
          screen.getByRole("button", { name: "Stop the 5 min timer" })
        )
        expect(screen.queryByTestId("cook-timer")).toBeNull()
      } finally {
        delete (navigator as { vibrate?: unknown }).vibrate
      }
    })
  })
})
