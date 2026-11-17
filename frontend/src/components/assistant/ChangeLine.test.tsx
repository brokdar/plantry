import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import "@/lib/i18n"
import { mockChange } from "@/test/fixtures"

import { ChangeLine } from "./ChangeLine"

const ops = ["Gurke", "Reis", "Dal", "Brot", "Tee"].map((food) => ({
  op: "add",
  food,
  date: "2026-11-03",
}))

describe("ChangeLine", () => {
  it("shows three ops and expands to the full list", async () => {
    const user = userEvent.setup()
    render(
      <ChangeLine
        change={mockChange({
          text_key: "apply_changes",
          params: { count: 5, ops },
        })}
        state={null}
        onUndo={vi.fn()}
      />
    )
    const text = screen.getByTestId("assistant-change-text")
    expect(text).toHaveTextContent("add Dal (Tuesday)")
    expect(text).not.toHaveTextContent("Brot")
    const more = screen.getByTestId("assistant-change-more")
    expect(more).toHaveTextContent("+2 more")
    expect(more).toHaveAttribute("aria-expanded", "false")

    await user.click(more)
    expect(text).toHaveTextContent("add Brot (Tuesday); add Tee (Tuesday)")
    expect(more).toHaveAttribute("aria-expanded", "true")
    expect(more).toHaveTextContent("show less")

    await user.click(more)
    expect(text).not.toHaveTextContent("Brot")
  })

  it("has no toggle for three ops or fewer", () => {
    render(
      <ChangeLine
        change={mockChange({
          text_key: "apply_changes",
          params: { count: 3, ops: ops.slice(0, 3) },
        })}
        state={null}
        onUndo={vi.fn()}
      />
    )
    expect(screen.queryByTestId("assistant-change-more")).toBeNull()
  })
})
