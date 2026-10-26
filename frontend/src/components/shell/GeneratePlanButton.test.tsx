import { beforeEach, describe, expect, test, vi } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"
import { useChatUI } from "@/lib/stores/chat-ui"
import { GeneratePlanButton } from "./GeneratePlanButton"

vi.mock("@/lib/api/setup", () => ({
  getSetupStatus: vi.fn(),
  completeSetup: vi.fn(),
}))

import { getSetupStatus } from "@/lib/api/setup"

function setupWith(recipe_count: number) {
  vi.mocked(getSetupStatus).mockResolvedValue({
    completed: true,
    recipe_count,
    recipe_target: 10,
  })
}

describe("GeneratePlanButton", () => {
  beforeEach(() => {
    vi.mocked(getSetupStatus).mockReset()
  })

  test("below the target it is aria-disabled, explains the gap and links to recipes", async () => {
    setupWith(4)
    const openWith = vi.fn()
    useChatUI.setState({ openWith })
    const user = userEvent.setup()
    renderWithRouter(<GeneratePlanButton />)

    const button = await screen.findByTestId("generate-plan-default")
    await vi.waitFor(() =>
      expect(button).toHaveAttribute("aria-disabled", "true")
    )
    // Still focusable: aria-disabled, not the disabled attribute.
    expect(button).not.toBeDisabled()
    const link = screen.getByRole("link", {
      name: "Add 6 more recipes to start planning",
    })
    expect(link).toHaveAttribute("href", "/components")
    expect(button).toHaveAccessibleDescription(
      "Add 6 more recipes to start planning"
    )

    await user.click(button)
    expect(openWith).not.toHaveBeenCalled()
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Add 6 more recipes to start planning"
    )
  })

  test("uses the singular for one missing recipe", async () => {
    setupWith(9)
    renderWithRouter(<GeneratePlanButton />)
    expect(
      await screen.findByRole("link", {
        name: "Add 1 more recipe to start planning",
      })
    ).toBeInTheDocument()
  })

  test("at the target it is enabled and opens the chat with the generate prompt", async () => {
    setupWith(10)
    const openWith = vi.fn()
    useChatUI.setState({ openWith })
    const user = userEvent.setup()
    renderWithRouter(<GeneratePlanButton />)

    const button = await screen.findByTestId("generate-plan-default")
    await vi.waitFor(() => expect(getSetupStatus).toHaveBeenCalled())
    expect(button).not.toHaveAttribute("aria-disabled")
    expect(screen.queryByRole("link", { name: /more recipe/ })).toBeNull()

    await user.click(button)
    await vi.waitFor(() =>
      expect(openWith).toHaveBeenCalledWith(
        expect.stringMatching(/plan my week/i)
      )
    )
  })

  test("the compact variant is an icon button with no visible link", async () => {
    setupWith(4)
    renderWithRouter(<GeneratePlanButton variant="compact" />)
    const button = await screen.findByTestId("generate-plan-compact")
    expect(button).toHaveAccessibleName("Generate Plan")
    await vi.waitFor(() =>
      expect(button).toHaveAttribute("aria-disabled", "true")
    )
    expect(screen.queryByRole("link")).toBeNull()
  })
})
