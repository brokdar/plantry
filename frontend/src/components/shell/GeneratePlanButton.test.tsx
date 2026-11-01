import { beforeEach, describe, expect, test, vi } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"
import { useChatUI } from "@/lib/stores/chat-ui"
import { GeneratePlanButton } from "./GeneratePlanButton"

const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigateMock,
}))

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
    navigateMock.mockReset()
    useChatUI.setState({ open: false, draftMessage: "" })
  })

  test("below the target it is aria-disabled, explains the gap and links to recipes", async () => {
    setupWith(4)
    const user = userEvent.setup()
    renderWithRouter(<GeneratePlanButton />)

    const button = await screen.findByTestId("generate-plan-default")
    const link = await screen.findByRole("link", {
      name: "Add 6 more recipes to start planning",
    })
    expect(button).toHaveAttribute("aria-disabled", "true")
    // Still focusable: aria-disabled, not the disabled attribute.
    expect(button).not.toBeDisabled()
    expect(link).toHaveAttribute("href", "/recipes")
    expect(button).toHaveAccessibleDescription(
      "Add 6 more recipes to start planning"
    )

    await user.click(button)
    expect(navigateMock).not.toHaveBeenCalled()
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

  test("F3-AC4: at the target it opens the plan wizard, never the chat", async () => {
    setupWith(10)
    const user = userEvent.setup()
    renderWithRouter(<GeneratePlanButton />)

    const button = await screen.findByTestId("generate-plan-default")
    await vi.waitFor(() => expect(button).not.toHaveAttribute("aria-disabled"))
    expect(screen.queryByRole("link", { name: /more recipe/ })).toBeNull()
    // on-accent on accent is ≥4.5:1 (tokens.test.ts); the size class must not
    // knock the colour out of the merged class list.
    expect(button).toHaveClass("bg-accent", "text-on-accent", "text-body")

    await user.click(button)
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan/new",
      search: { start: undefined },
    })
    expect(useChatUI.getState().open).toBe(false)
    expect(useChatUI.getState().draftMessage).toBe("")
  })

  test("on the board it plans the window the board shows", async () => {
    setupWith(10)
    const user = userEvent.setup()
    renderWithRouter(<GeneratePlanButton />, "/plan?start=2026-11-07")

    const button = await screen.findByTestId("generate-plan-default")
    await vi.waitFor(() => expect(button).not.toHaveAttribute("aria-disabled"))
    await user.click(button)
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan/new",
      search: { start: "2026-11-07" },
    })
  })

  test("until the status loads it is locked, without a count or link", async () => {
    vi.mocked(getSetupStatus).mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()
    renderWithRouter(<GeneratePlanButton />)

    const button = await screen.findByTestId("generate-plan-default")
    expect(button).toHaveAttribute("aria-disabled", "true")
    expect(button).not.toHaveAccessibleDescription()
    expect(screen.queryByRole("link")).toBeNull()

    await user.click(button)
    expect(navigateMock).not.toHaveBeenCalled()
    expect(screen.queryByRole("tooltip")).toBeNull()
  })

  test("the compact variant is an icon button with no visible link", async () => {
    setupWith(4)
    renderWithRouter(<GeneratePlanButton variant="compact" />)
    const button = await screen.findByTestId("generate-plan-compact")
    expect(button).toHaveAccessibleName("Generate plan")
    await vi.waitFor(() =>
      expect(button).toHaveAccessibleDescription(
        "Add 6 more recipes to start planning"
      )
    )
    expect(button).toHaveAttribute("aria-disabled", "true")
    expect(screen.queryByRole("link")).toBeNull()
  })
})
