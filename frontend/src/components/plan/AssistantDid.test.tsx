import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/assistant-did", () => ({
  getAssistantDid: vi.fn(),
  undoPlacedFind: vi.fn(),
  planOpened: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({
  toast: Object.assign(vi.fn(), { success: vi.fn() }),
  toastError: vi.fn(),
}))

import { getAssistantDid, undoPlacedFind } from "@/lib/api/assistant-did"

import { AssistantDid } from "./AssistantDid"

describe("AssistantDid", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
    vi.mocked(undoPlacedFind).mockResolvedValue(undefined)
  })

  it("lists what the assistant placed and undoes one entry", async () => {
    vi.mocked(getAssistantDid).mockResolvedValue({
      items: [{ kind: "find_placed", find_id: 4, food_id: 9, name: "Dal" }],
      more: 2,
    })
    renderWithRouter(<AssistantDid start="2026-11-23" enabled />)
    expect(
      await screen.findByText("Added Dal to this week")
    ).toBeInTheDocument()
    expect(screen.getByText("and 2 more")).toBeInTheDocument()
    await userEvent.click(screen.getByRole("button", { name: "Undo: Dal" }))
    await waitFor(() => expect(undoPlacedFind).toHaveBeenCalled())
    expect(vi.mocked(undoPlacedFind).mock.calls[0][0]).toBe(4)
  })

  it("shows nothing when empty or with the Chef off", async () => {
    vi.mocked(getAssistantDid).mockResolvedValue({ items: [], more: 0 })
    const { container } = renderWithRouter(
      <AssistantDid start="2026-11-23" enabled={false} />
    )
    expect(getAssistantDid).not.toHaveBeenCalled()
    expect(container.querySelector("[data-testid=assistant-did]")).toBeNull()
  })
})
