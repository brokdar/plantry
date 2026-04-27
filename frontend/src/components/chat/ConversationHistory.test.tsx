import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, test, vi } from "vitest"

import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/queries/ai", () => ({
  useConversations: vi.fn(),
  useDeleteConversation: vi.fn(),
}))

import { useConversations, useDeleteConversation } from "@/lib/queries/ai"
import { useChatUI } from "@/lib/stores/chat-ui"

import { ConversationHistory } from "./ConversationHistory"

function stubConversations(
  items: Array<{ id: number; title?: string; created_at: string }>
) {
  vi.mocked(useConversations).mockReturnValue({
    data: { items, total: items.length },
  } as unknown as ReturnType<typeof useConversations>)
  const mutate = vi.fn()
  vi.mocked(useDeleteConversation).mockReturnValue({
    mutate,
  } as unknown as ReturnType<typeof useDeleteConversation>)
  return { mutate }
}

beforeEach(() => {
  vi.clearAllMocks()
  useChatUI.setState({ activeConversationId: null })
})

describe("ConversationHistory", () => {
  test("renders empty state when no conversations exist", async () => {
    stubConversations([])
    renderWithRouter(<ConversationHistory />)

    await userEvent.click(
      await screen.findByRole("button", { name: /Open history/ })
    )
    expect(await screen.findByText("No conversations yet.")).toBeInTheDocument()
  })

  test("selecting a conversation sets it active", async () => {
    stubConversations([
      { id: 7, title: "planning Tuesday", created_at: "2024-01-01T12:00:00Z" },
    ])
    renderWithRouter(<ConversationHistory />)

    await userEvent.click(
      await screen.findByRole("button", { name: /Open history/ })
    )
    await userEvent.click(
      await screen.findByRole("button", { name: /Open conversation/ })
    )
    expect(useChatUI.getState().activeConversationId).toBe(7)
  })

  test("delete button triggers mutation", async () => {
    const { mutate } = stubConversations([
      { id: 9, title: "old chat", created_at: "2024-01-01T12:00:00Z" },
    ])
    renderWithRouter(<ConversationHistory />)

    await userEvent.click(
      await screen.findByRole("button", { name: /Open history/ })
    )
    await userEvent.click(
      await screen.findByRole("button", { name: "Delete conversation" })
    )
    expect(mutate).toHaveBeenCalledWith(9, expect.any(Object))
  })

  test("falls back to date when no title", async () => {
    stubConversations([{ id: 11, created_at: "2024-03-15T09:30:00Z" }])
    renderWithRouter(<ConversationHistory />)

    await userEvent.click(
      await screen.findByRole("button", { name: /Open history/ })
    )
    // Label falls back to a formatted date — assert the item is rendered.
    expect(
      await screen.findByTestId("chat-history-item-11")
    ).toBeInTheDocument()
  })
})
