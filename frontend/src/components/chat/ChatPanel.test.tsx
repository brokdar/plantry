import { act, render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, test, vi } from "vitest"

import "@/lib/i18n"

vi.mock("@/lib/queries/ai", () => ({
  useAISettings: vi.fn(),
  useChatStream: vi.fn(),
  useConversation: vi.fn(),
  useConversations: vi.fn(),
  useDeleteConversation: vi.fn(),
}))

import {
  useAISettings,
  useChatStream,
  useConversation,
  useConversations,
  useDeleteConversation,
} from "@/lib/queries/ai"
import { chatStreamStore } from "@/lib/stores/chat-stream"
import { useChatUI } from "@/lib/stores/chat-ui"

import { ChatPanel } from "./ChatPanel"

function stubQueries(opts: {
  enabled: boolean
  isStreaming?: boolean
  isError?: boolean
  messages?: { id: number; role: "user" | "assistant"; text: string }[]
}) {
  const mutateAsync = vi.fn().mockResolvedValue(undefined)
  vi.mocked(useAISettings).mockReturnValue({
    data: { enabled: opts.enabled, model: "test-model" },
  } as unknown as ReturnType<typeof useAISettings>)
  vi.mocked(useChatStream).mockReturnValue({
    mutateAsync,
    abort: vi.fn(),
    isStreaming: opts.isStreaming ?? false,
    isError: opts.isError ?? false,
  } as unknown as ReturnType<typeof useChatStream>)
  vi.mocked(useConversation).mockReturnValue({
    data: opts.messages && {
      messages: opts.messages.map((m) => ({
        id: m.id,
        role: m.role,
        content: [{ type: "text", text: m.text }],
        created_at: "2025-06-10T12:00:00Z",
      })),
    },
  } as unknown as ReturnType<typeof useConversation>)
  vi.mocked(useConversations).mockReturnValue({
    data: { items: [], total: 0 },
  } as unknown as ReturnType<typeof useConversations>)
  vi.mocked(useDeleteConversation).mockReturnValue({
    mutate: vi.fn(),
  } as unknown as ReturnType<typeof useDeleteConversation>)
  return { mutateAsync }
}

const range = { from: "2025-06-09", to: "2025-06-15" }

beforeEach(() => {
  vi.clearAllMocks()
  act(() => {
    chatStreamStore.reset()
    useChatUI.setState({
      open: false,
      activeConversationId: null,
    })
  })
})

describe("ChatPanel composer", () => {
  test("sends the typed message for the window, without a mode", async () => {
    const { mutateAsync } = stubQueries({ enabled: true })
    act(() => {
      useChatUI.setState({ open: true, draftMessage: "plan my dinner" })
    })

    render(<ChatPanel range={range} />)

    expect(screen.queryByTestId("chat-composer-mode")).not.toBeInTheDocument()
    act(() => {
      screen.getByTestId("chat-composer-submit").click()
    })
    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        conversationId: undefined,
        range,
        message: "plan my dinner",
      })
    })
  })
})

describe("ChatPanel transcript", () => {
  function openPanel() {
    act(() => {
      useChatUI.setState({ open: true, activeConversationId: 1 })
      chatStreamStore.appendText("Dinner is planned.")
    })
  }

  test("shows the final reply once after the stream completes", async () => {
    stubQueries({
      enabled: true,
      messages: [
        { id: 1, role: "user", text: "plan my dinner" },
        { id: 2, role: "assistant", text: "Dinner is planned." },
      ],
    })
    openPanel()

    render(<ChatPanel range={range} />)

    expect(await screen.findByText("Dinner is planned.")).toBeInTheDocument()
    expect(screen.getAllByText("Dinner is planned.")).toHaveLength(1)
  })

  test("shows the streamed text while the reply is still streaming", async () => {
    stubQueries({
      enabled: true,
      isStreaming: true,
      messages: [{ id: 1, role: "user", text: "plan my dinner" }],
    })
    openPanel()

    render(<ChatPanel range={range} />)

    expect(await screen.findByText("Dinner is planned.")).toBeInTheDocument()
  })

  test("keeps the streamed text when the stream failed", async () => {
    stubQueries({
      enabled: true,
      isError: true,
      messages: [{ id: 1, role: "user", text: "plan my dinner" }],
    })
    openPanel()

    render(<ChatPanel range={range} />)

    expect(await screen.findByText("Dinner is planned.")).toBeInTheDocument()
  })
})
