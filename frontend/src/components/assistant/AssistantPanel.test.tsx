import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, test, vi } from "vitest"

import "@/lib/i18n"

vi.mock("@/lib/queries/ai", () => ({
  useConversationTitles: vi.fn(() => new Map()),
  useAssistantStream: vi.fn(),
  useConversation: vi.fn(),
  useConversations: vi.fn(),
  useDeleteConversation: vi.fn(),
  useUndoChange: vi.fn(),
}))
vi.mock("@/lib/queries/settings", () => ({
  useSettingItems: () => ({ data: {} }),
}))
vi.mock("@/lib/queries/foods", () => ({
  useFood: () => ({ data: undefined }),
}))

import {
  useAssistantStream,
  useConversation,
  useConversations,
  useDeleteConversation,
  useUndoChange,
} from "@/lib/queries/ai"
import { assistantStream } from "@/lib/stores/assistant-stream"
import type { ConversationChange } from "@/lib/domain/assistantEvents"
import { mockChange, mockPlan } from "@/test/fixtures"

import { AssistantPanel } from "./AssistantPanel"

function stubQueries(opts: {
  isStreaming?: boolean
  isError?: boolean
  messages?: {
    id: number
    role: "user" | "assistant"
    text: string
    toolUse?: { id: string; name: string }
  }[]
  changes?: ConversationChange[]
  /** Stored messages as the API returns them, instead of `messages`. */
  raw?: unknown[]
  conversations?: { id: number; created_at: string; updated_at: string }[]
}) {
  const undo = vi.fn()
  vi.mocked(useUndoChange).mockReturnValue({
    mutate: undo,
  } as unknown as ReturnType<typeof useUndoChange>)
  const mutateAsync = vi.fn().mockResolvedValue(undefined)
  vi.mocked(useAssistantStream).mockReturnValue({
    mutateAsync,
    abort: vi.fn(),
    isStreaming: opts.isStreaming ?? false,
    isError: opts.isError ?? false,
  } as unknown as ReturnType<typeof useAssistantStream>)
  vi.mocked(useConversation).mockReturnValue({
    data: (opts.raw || opts.messages) && {
      messages:
        opts.raw ??
        opts.messages!.map((m) => ({
          id: m.id,
          role: m.role,
          content: [
            { type: "text", text: m.text },
            ...(m.toolUse
              ? [
                  {
                    type: "tool_use",
                    tool_use_id: m.toolUse.id,
                    tool_use_name: m.toolUse.name,
                  },
                ]
              : []),
          ],
          created_at: "2025-06-10T12:00:00Z",
        })),
      changes: opts.changes ?? [],
    },
  } as unknown as ReturnType<typeof useConversation>)
  vi.mocked(useConversations).mockReturnValue({
    data: { items: opts.conversations ?? [], total: 0 },
  } as unknown as ReturnType<typeof useConversations>)
  vi.mocked(useDeleteConversation).mockReturnValue({
    mutate: vi.fn(),
  } as unknown as ReturnType<typeof useDeleteConversation>)
  return { mutateAsync, undo }
}

function renderPanel(
  props: Partial<React.ComponentProps<typeof AssistantPanel>> = {}
) {
  const onClose = vi.fn()
  render(
    <AssistantPanel
      open
      phone={false}
      enabled
      plan={mockPlan}
      onClose={onClose}
      {...props}
    />
  )
  return { onClose }
}

beforeEach(() => {
  vi.clearAllMocks()
  act(() => {
    assistantStream.clear()
  })
})

describe("AssistantPanel shell", () => {
  test("is an aside, not a dialog, from 768 px", () => {
    stubQueries({})
    renderPanel()
    expect(
      screen.getByRole("complementary", { name: "Assistant" })
    ).toBeInTheDocument()
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
  })

  test("is a modal sheet on a phone", async () => {
    stubQueries({})
    renderPanel({ phone: true })
    expect(await screen.findByRole("dialog")).toBeInTheDocument()
  })

  test("Escape inside the aside and the close button close it", async () => {
    stubQueries({})
    const user = userEvent.setup()
    const { onClose } = renderPanel()
    await user.keyboard("{Escape}")
    expect(onClose).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole("button", { name: "Close assistant" }))
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  test("renders nothing when closed", () => {
    stubQueries({})
    renderPanel({ open: false })
    expect(screen.queryByTestId("assistant-panel")).not.toBeInTheDocument()
  })

  test("without a provider it says so and the composer is disabled", () => {
    stubQueries({})
    renderPanel({ enabled: false })
    expect(screen.getByText(/No AI provider/)).toBeInTheDocument()
    expect(screen.getByTestId("assistant-composer-input")).toBeDisabled()
    expect(screen.queryByTestId("assistant-chips")).not.toBeInTheDocument()
  })
})

describe("AssistantPanel suggestion chips", () => {
  test("a chip fills the composer and does not send", async () => {
    const { mutateAsync } = stubQueries({})
    const user = userEvent.setup()
    renderPanel()
    await user.click(screen.getByRole("button", { name: "No fish this week" }))
    expect(screen.getByTestId("assistant-composer-input")).toHaveValue(
      "No fish this week"
    )
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  test("are gone once the conversation has turns", () => {
    stubQueries({ messages: [{ id: 1, role: "user", text: "hi" }] })
    act(() => assistantStream.setConversation(1))
    renderPanel()
    expect(screen.queryByTestId("assistant-chips")).not.toBeInTheDocument()
  })
})

describe("AssistantPanel after a reload", () => {
  const preview = [
    {
      id: 2,
      role: "assistant",
      content: [
        { type: "text", text: "Please say yes." },
        { type: "tool_use", tool_use_id: "p", tool_use_name: "unlock" },
      ],
      created_at: "2025-06-10T12:00:00Z",
    },
    {
      id: 3,
      role: "tool",
      content: [
        {
          type: "tool_result",
          tool_result_id: "p",
          tool_result_content: JSON.stringify({
            status: "needs_user_confirmation",
            rows: [{ date: "2026-10-31", slot_id: 3, names: ["Chili"] }],
          }),
        },
      ],
      created_at: "2025-06-10T12:00:00Z",
    },
  ]

  test("reopens the most recent conversation when it opens with the panel", () => {
    stubQueries({
      raw: preview,
      conversations: [
        {
          id: 4,
          created_at: "2025-06-09T10:00:00Z",
          updated_at: "2025-06-09T10:00:00Z",
        },
        {
          id: 9,
          created_at: "2025-06-10T10:00:00Z",
          updated_at: "2025-06-10T10:00:00Z",
        },
      ],
    })
    renderPanel()
    expect(assistantStream.get().conversationId).toBe(9)
  })

  test("does not pick a conversation when the panel is opened later", () => {
    stubQueries({
      conversations: [
        {
          id: 4,
          created_at: "2025-06-09T10:00:00Z",
          updated_at: "2025-06-09T10:00:00Z",
        },
      ],
    })
    renderPanel({ open: false })
    expect(assistantStream.get().conversationId).toBeNull()
  })

  test("a stored unlock preview nobody answered shows the card", () => {
    stubQueries({ raw: preview })
    act(() => assistantStream.setConversation(1))
    renderPanel()
    expect(screen.getByTestId("assistant-unlock-card")).toHaveTextContent(
      "Chili"
    )
  })

  test("the card is gone once the user answered", () => {
    stubQueries({
      raw: [
        ...preview,
        {
          id: 4,
          role: "user",
          content: [{ type: "text", text: "Yes, unlock" }],
          created_at: "2025-06-10T12:01:00Z",
        },
      ],
    })
    act(() => assistantStream.setConversation(1))
    renderPanel()
    expect(
      screen.queryByTestId("assistant-unlock-card")
    ).not.toBeInTheDocument()
  })
})

describe("AssistantPanel composer", () => {
  test("sends the typed message for the window, without a mode", async () => {
    const { mutateAsync } = stubQueries({})
    act(() => {
      assistantStream.setDraft("plan my dinner")
    })
    renderPanel()
    act(() => {
      screen.getByTestId("assistant-composer-submit").click()
    })
    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        conversationId: undefined,
        start: mockPlan.window.start,
        message: "plan my dinner",
      })
    })
  })
})

describe("AssistantPanel transcript", () => {
  function openConversation() {
    act(() => {
      assistantStream.setConversation(1)
      assistantStream.appendText("Dinner is planned.")
    })
  }

  test("shows the final reply once after the stream completes", async () => {
    stubQueries({
      messages: [
        { id: 1, role: "user", text: "plan my dinner" },
        { id: 2, role: "assistant", text: "Dinner is planned." },
      ],
    })
    openConversation()
    renderPanel()
    expect(await screen.findByText("Dinner is planned.")).toBeInTheDocument()
    expect(screen.getAllByText("Dinner is planned.")).toHaveLength(1)
  })

  test("shows the streamed text and the status line while streaming", async () => {
    stubQueries({
      isStreaming: true,
      messages: [{ id: 1, role: "user", text: "plan my dinner" }],
    })
    openConversation()
    act(() => assistantStream.setStatus("get_plan"))
    renderPanel()
    expect(await screen.findByText("Dinner is planned.")).toBeInTheDocument()
    expect(screen.getByTestId("assistant-status")).toHaveTextContent(
      "Reading your plan…"
    )
    expect(screen.queryByText(/get_plan/)).not.toBeInTheDocument()
  })

  test("keeps the streamed text when the stream failed", async () => {
    stubQueries({
      isError: true,
      messages: [{ id: 1, role: "user", text: "plan my dinner" }],
    })
    openConversation()
    renderPanel()
    expect(await screen.findByText("Dinner is planned.")).toBeInTheDocument()
  })
})

describe("AssistantPanel changes (F3-AC4)", () => {
  const live = () => ({ ...mockChange(), undone_at: null })
  const oneChange = {
    messages: [
      { id: 1, role: "user" as const, text: "chili 6" },
      {
        id: 2,
        role: "assistant" as const,
        text: "Done.",
        toolUse: { id: "tu_1", name: "set_session_portions" },
      },
    ],
  }

  test("a failed tool call shows one failure line, not a change", () => {
    stubQueries({
      raw: [
        {
          id: 2,
          role: "assistant",
          content: [
            {
              type: "tool_use",
              tool_use_id: "r1",
              tool_use_name: "regenerate",
            },
          ],
          created_at: "2025-06-10T12:00:00Z",
        },
        {
          id: 3,
          role: "tool",
          content: [
            {
              type: "tool_result",
              tool_result_id: "r1",
              tool_result_is_error: true,
            },
          ],
          created_at: "2025-06-10T12:00:00Z",
        },
      ],
    })
    act(() => assistantStream.setConversation(1))
    renderPanel()
    expect(screen.getAllByTestId("assistant-failure")).toHaveLength(1)
    expect(screen.getByTestId("assistant-failure")).toHaveTextContent(
      "Nothing was changed"
    )
    expect(screen.queryByTestId("assistant-change")).not.toBeInTheDocument()
  })

  test("a stale undo shows 'Changed since' and no button", () => {
    stubQueries({ ...oneChange, changes: [live()] })
    act(() => {
      assistantStream.setConversation(1)
      assistantStream.setLineState(5, "stale")
    })
    renderPanel()
    const line = screen.getByTestId("assistant-change")
    expect(line).toHaveTextContent("Chili: 6 portions")
    expect(line).toHaveTextContent("Changed since")
    expect(
      screen.queryByTestId("assistant-change-undo")
    ).not.toBeInTheDocument()
  })

  test("a change that could not be saved says so and has no Undo", () => {
    stubQueries({ ...oneChange })
    act(() => {
      assistantStream.setConversation(1)
      assistantStream.addChange(
        mockChange({ id: 0, recorded: false, undoable: false })
      )
    })
    renderPanel()
    const line = screen.getByTestId("assistant-change")
    expect(line).toHaveTextContent("Chili: 6 portions")
    expect(screen.getByTestId("assistant-change-unrecorded")).toHaveTextContent(
      "Couldn't be saved, undo not possible"
    )
    expect(
      screen.queryByTestId("assistant-change-undo")
    ).not.toBeInTheDocument()
  })

  test("a change with nothing to undo ends in 'Nothing to undo'", () => {
    stubQueries({ ...oneChange, changes: [live()] })
    act(() => {
      assistantStream.setConversation(1)
      assistantStream.setLineState(5, "nothing_to_undo")
    })
    renderPanel()
    expect(screen.getByTestId("assistant-change-nothing")).toHaveTextContent(
      "Nothing to undo"
    )
    expect(
      screen.queryByTestId("assistant-change-undo")
    ).not.toBeInTheDocument()
  })

  test("a change line has Undo that undoes that change", async () => {
    const { undo } = stubQueries({ ...oneChange, changes: [live()] })
    const user = userEvent.setup()
    act(() => assistantStream.setConversation(1))
    renderPanel()
    await user.click(screen.getByTestId("assistant-change-undo"))
    expect(undo).toHaveBeenCalledWith(5)
  })

  test("a persisted undone change shows 'Undone' without a button", () => {
    stubQueries({
      ...oneChange,
      changes: [{ ...mockChange(), undone_at: "2026-11-01T09:04:00Z" }],
    })
    act(() => assistantStream.setConversation(1))
    renderPanel()
    expect(screen.getByTestId("assistant-change")).toHaveTextContent("Undone")
    expect(
      screen.queryByTestId("assistant-change-undo")
    ).not.toBeInTheDocument()
  })

  test("the live region reads the change sentence", () => {
    stubQueries({})
    act(() => {
      assistantStream.addChange(
        mockChange({ params: { name: "Chili", portions: 6, frozen: 2 } })
      )
    })
    renderPanel()
    expect(screen.getByTestId("assistant-live")).toHaveTextContent(
      "Chili: 6 portions, 2 frozen"
    )
    expect(screen.getByTestId("assistant-change-text")).toHaveTextContent(
      "Chili: 6 portions, 2 frozen"
    )
  })

  test("no element shows a raw tool name", () => {
    stubQueries({ ...oneChange, changes: [live()] })
    act(() => {
      assistantStream.setConversation(1)
      assistantStream.setStatus("set_session_portions")
    })
    renderPanel({})
    expect(screen.queryByText(/set_session_portions/)).not.toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/set_session_portions/)
  })

  test("an unlock request shows the card and Yes, unlock sends that text", async () => {
    const { mutateAsync } = stubQueries({})
    const user = userEvent.setup()
    act(() => {
      assistantStream.setUnlockRequest({
        rows: [{ date: "2026-11-02", slot_id: 3, names: ["Chili", "Rice"] }],
      })
    })
    renderPanel()
    expect(screen.getByTestId("assistant-unlock-card")).toHaveTextContent(
      "Monday dinner (Chili, Rice)"
    )
    await user.click(screen.getByRole("button", { name: "Yes, unlock" }))
    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ message: "Yes, unlock" })
    )
  })
})
