import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import "@/lib/i18n"

import { ToolCallBlock } from "./ToolCallBlock"

describe("ToolCallBlock", () => {
  it("renders the tool name", () => {
    render(
      <ToolCallBlock
        tool={{
          id: "tu_1",
          name: "create_plate",
          argsJson: "{}",
          status: "running",
        }}
      />
    )
    expect(screen.getByText("create_plate")).toBeInTheDocument()
  })

  it("reflects status in data-state", () => {
    const { rerender } = render(
      <ToolCallBlock
        tool={{
          id: "tu_1",
          name: "create_plate",
          argsJson: "",
          status: "running",
        }}
      />
    )
    expect(
      screen.getByTestId("chat-tool-call").getAttribute("data-state")
    ).toBe("running")

    rerender(
      <ToolCallBlock
        tool={{
          id: "tu_1",
          name: "create_plate",
          argsJson: "",
          status: "ok",
          durationMs: 15,
        }}
      />
    )
    expect(
      screen.getByTestId("chat-tool-call").getAttribute("data-state")
    ).toBe("ok")
    expect(screen.getByText(/15 ms/)).toBeInTheDocument()
  })

  it("shows collapsible args", () => {
    render(
      <ToolCallBlock
        tool={{
          id: "tu_1",
          name: "create_plate",
          argsJson: '{"day":0}',
          status: "ok",
        }}
      />
    )
    expect(screen.getByText('{"day":0}')).toBeInTheDocument()
  })
})
