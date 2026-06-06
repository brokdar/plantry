import { act, render, waitFor } from "@testing-library/react"
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
import { useChatUI } from "@/lib/stores/chat-ui"

import { ChatPanel } from "./ChatPanel"

function stubQueries(opts: { enabled: boolean }) {
  const mutateAsync = vi.fn().mockResolvedValue(undefined)
  vi.mocked(useAISettings).mockReturnValue({
    data: { enabled: opts.enabled, model: "test-model" },
  } as unknown as ReturnType<typeof useAISettings>)
  vi.mocked(useChatStream).mockReturnValue({
    mutateAsync,
    abort: vi.fn(),
    isStreaming: false,
  } as unknown as ReturnType<typeof useChatStream>)
  vi.mocked(useConversation).mockReturnValue({
    data: undefined,
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
    useChatUI.setState({
      open: false,
      mode: "",
      autoSend: null,
      activeConversationId: null,
    })
  })
})

describe("ChatPanel auto-send", () => {
  test("auto-submits the queued fill message in fill_empty mode", async () => {
    const { mutateAsync } = stubQueries({ enabled: true })
    act(() => {
      useChatUI.getState().requestAutoFill("fill my week")
    })

    render(<ChatPanel range={range} />)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        conversationId: undefined,
        range,
        mode: "fill_empty",
        message: "fill my week",
      })
    })
    // Fires exactly once and clears the queue so it never re-sends.
    expect(mutateAsync).toHaveBeenCalledTimes(1)
    expect(useChatUI.getState().autoSend).toBeNull()
  })

  test("does not auto-send when AI is disabled", async () => {
    const { mutateAsync } = stubQueries({ enabled: false })
    act(() => {
      useChatUI.getState().requestAutoFill("fill my week")
    })

    render(<ChatPanel range={range} />)

    // Give the effect a chance to run, then confirm it stayed put.
    await Promise.resolve()
    expect(mutateAsync).not.toHaveBeenCalled()
    expect(useChatUI.getState().autoSend).toBe("fill my week")
  })
})
