import { act, fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import "@/lib/i18n"
import { assistantStream } from "@/lib/stores/assistant-stream"

import { Composer } from "./Composer"

describe("Composer", () => {
  beforeEach(() => {
    act(() => {
      assistantStream.clear()
    })
  })

  it("submits on click when there's text", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(<Composer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />)
    await user.type(screen.getByTestId("assistant-composer-input"), "hello")
    await user.click(screen.getByTestId("assistant-composer-submit"))
    expect(onSubmit).toHaveBeenCalledWith("hello")
  })

  it("submits on ctrl+enter", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(<Composer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />)
    const input = screen.getByTestId("assistant-composer-input")
    await user.type(input, "hello")
    await user.keyboard("{Control>}{Enter}{/Control}")
    expect(onSubmit).toHaveBeenCalledWith("hello")
  })

  it("submits on cmd+enter", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(<Composer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />)
    await user.type(screen.getByTestId("assistant-composer-input"), "hello")
    await user.keyboard("{Meta>}{Enter}{/Meta}")
    expect(onSubmit).toHaveBeenCalledWith("hello")
  })

  it("does not send on the Enter that confirms an IME composition", () => {
    const onSubmit = vi.fn()
    render(<Composer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />)
    const input = screen.getByTestId("assistant-composer-input")
    fireEvent.change(input, { target: { value: "こんにちは" } })
    fireEvent.keyDown(input, { key: "Enter", isComposing: true })
    expect(onSubmit).not.toHaveBeenCalled()
    fireEvent.keyDown(input, { key: "Enter", isComposing: false })
    expect(onSubmit).toHaveBeenCalledWith("こんにちは")
  })

  it("sends on Enter and keeps Shift+Enter as a new line", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(<Composer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />)
    const input = screen.getByTestId("assistant-composer-input")
    await user.type(input, "line one{Shift>}{Enter}{/Shift}line two")
    expect(onSubmit).not.toHaveBeenCalled()
    expect(input).toHaveValue("line one\nline two")
    await user.keyboard("{Enter}")
    expect(onSubmit).toHaveBeenCalledWith("line one\nline two")
  })

  it("has no mode select", () => {
    render(<Composer streaming={false} onSubmit={vi.fn()} onAbort={vi.fn()} />)
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument()
    expect(
      screen.queryByTestId("assistant-composer-mode")
    ).not.toBeInTheDocument()
  })

  it("does not submit empty text", async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(<Composer streaming={false} onSubmit={onSubmit} onAbort={vi.fn()} />)
    const submit = screen.getByTestId("assistant-composer-submit")
    expect(submit).toBeDisabled()
    await user.click(submit)
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it("shows abort button while streaming and calls onAbort", async () => {
    const onAbort = vi.fn()
    const user = userEvent.setup()
    render(<Composer streaming={true} onSubmit={vi.fn()} onAbort={onAbort} />)
    await user.click(screen.getByTestId("assistant-composer-abort"))
    expect(onAbort).toHaveBeenCalled()
  })

  it("is disabled when prop disabled=true", () => {
    render(
      <Composer
        streaming={false}
        onSubmit={vi.fn()}
        onAbort={vi.fn()}
        disabled
      />
    )
    expect(screen.getByTestId("assistant-composer-input")).toBeDisabled()
  })
})
