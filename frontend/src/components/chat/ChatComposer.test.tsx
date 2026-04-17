import { act, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import "@/lib/i18n"
import { useChatUI } from "@/lib/stores/chat-ui"

import { ChatComposer } from "./ChatComposer"

describe("ChatComposer", () => {
  beforeEach(() => {
    act(() => {
      useChatUI.getState().setDraft("")
      useChatUI.getState().setOpen(false)
      useChatUI.getState().setStreaming(false)
    })
  })

  it("submits on click when there's text", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatComposer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />
    )
    await user.type(screen.getByTestId("chat-composer-input"), "hello")
    await user.click(screen.getByTestId("chat-composer-submit"))
    expect(onSubmit).toHaveBeenCalledWith("hello", "")
  })

  it("submits on ctrl+enter", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatComposer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />
    )
    const input = screen.getByTestId("chat-composer-input")
    await user.type(input, "hello")
    await user.keyboard("{Control>}{Enter}{/Control}")
    expect(onSubmit).toHaveBeenCalledWith("hello", "")
  })

  it("pipes the selected mode through onSubmit", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatComposer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />
    )
    await user.selectOptions(
      screen.getByTestId("chat-composer-mode"),
      "fill_empty"
    )
    await user.type(screen.getByTestId("chat-composer-input"), "plan")
    await user.click(screen.getByTestId("chat-composer-submit"))
    expect(onSubmit).toHaveBeenCalledWith("plan", "fill_empty")
  })

  it("does not submit empty text", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatComposer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />
    )
    const submit = screen.getByTestId("chat-composer-submit")
    expect(submit).toBeDisabled()
    await user.click(submit)
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it("shows abort button while streaming and calls onAbort", async () => {
    const onAbort = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatComposer streaming={true} onSubmit={vi.fn()} onAbort={onAbort} />
    )
    await user.click(screen.getByTestId("chat-composer-abort"))
    expect(onAbort).toHaveBeenCalled()
  })

  it("is disabled when prop disabled=true", () => {
    render(
      <ChatComposer
        streaming={false}
        onSubmit={vi.fn()}
        onAbort={vi.fn()}
        disabled
      />
    )
    expect(screen.getByTestId("chat-composer-input")).toBeDisabled()
  })
})
