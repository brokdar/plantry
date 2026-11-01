import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, test, vi } from "vitest"

import "@/lib/i18n"

vi.mock("@/lib/queries/ai", () => ({
  useAssistantStream: vi.fn(),
  useConversation: vi.fn(),
  useConversations: vi.fn(),
  useDeleteConversation: vi.fn(),
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
} from "@/lib/queries/ai"
import { assistantStream } from "@/lib/stores/assistant-stream"
import { mockPlan } from "@/test/fixtures"

import { AssistantPanel } from "./AssistantPanel"

function stubQueries(opts: {
  isStreaming?: boolean
  isError?: boolean
  messages?: { id: number; role: "user" | "assistant"; text: string }[]
}) {
  const mutateAsync = vi.fn().mockResolvedValue(undefined)
  vi.mocked(useAssistantStream).mockReturnValue({
    mutateAsync,
    abort: vi.fn(),
    isStreaming: opts.isStreaming ?? false,
    isError: opts.isError ?? false,
  } as unknown as ReturnType<typeof useAssistantStream>)
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
    expect(screen.getByRole("status")).toHaveTextContent("Using get_plan")
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
