import { cleanup, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { mockAvailabilityPresets } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

import { RoutinePicker } from "./RoutinePicker"

function renderPicker(
  props: Partial<React.ComponentProps<typeof RoutinePicker>>
) {
  const onPick = vi.fn()
  const onOpenChange = vi.fn()
  renderWithRouter(
    <RoutinePicker
      open
      onOpenChange={onOpenChange}
      presets={mockAvailabilityPresets}
      onPick={onPick}
      {...props}
    >
      <button type="button">Anchor</button>
    </RoutinePicker>
  )
  return { onPick, onOpenChange }
}

describe("RoutinePicker", () => {
  // Radix FocusScope restores focus in a setTimeout(0) when the open popover
  // unmounts. Unmount here and let that timer run before the test ends, so it
  // never fires after the environment is torn down.
  afterEach(async () => {
    cleanup()
    await new Promise((resolve) => setTimeout(resolve, 0))
  })

  it("lists presets and picks one", async () => {
    const user = userEvent.setup()
    const { onPick } = renderPicker({})
    const picker = await screen.findByRole("dialog", { name: "Pick a routine" })
    await user.click(
      within(picker).getByRole("button", { name: "Overnight oats" })
    )
    expect(onPick).toHaveBeenCalledWith(7)
  })

  it("opens with focus on the first routine", async () => {
    renderPicker({})
    const picker = await screen.findByRole("dialog", { name: "Pick a routine" })
    await vi.waitFor(() =>
      expect(
        within(picker).getByRole("button", { name: "Overnight oats" })
      ).toHaveFocus()
    )
  })

  it("marks the current preset", async () => {
    renderPicker({ selectedId: 8 })
    const picker = await screen.findByRole("dialog", { name: "Pick a routine" })
    expect(
      within(picker).getByRole("button", { name: "Bread and soup" })
    ).toHaveAttribute("aria-pressed", "true")
    expect(
      within(picker).getByRole("button", { name: "Overnight oats" })
    ).toHaveAttribute("aria-pressed", "false")
  })

  it("cancel closes without picking", async () => {
    const user = userEvent.setup()
    const { onPick, onOpenChange } = renderPicker({})
    const picker = await screen.findByRole("dialog", { name: "Pick a routine" })
    await user.click(within(picker).getByRole("button", { name: "Cancel" }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(onPick).not.toHaveBeenCalled()
  })

  it("links to routines when there are none", async () => {
    renderPicker({ presets: [] })
    const picker = await screen.findByRole("dialog", { name: "Pick a routine" })
    expect(within(picker).getByText("No routines saved yet.")).toBeVisible()
    expect(
      within(picker).getByRole("link", { name: "Create a routine" })
    ).toHaveAttribute("href", "/settings/meals")
  })
})
