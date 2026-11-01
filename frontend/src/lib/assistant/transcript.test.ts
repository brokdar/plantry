import { describe, expect, it } from "vitest"

import type { ConversationMessage } from "@/lib/domain/assistantEvents"
import { mockChange } from "@/test/fixtures"

import { buildTranscript } from "./transcript"

const at = "2026-11-01T09:00:00Z"
const msg = (
  id: number,
  role: ConversationMessage["role"],
  content: unknown[]
): ConversationMessage => ({ id, role, content, created_at: at })
const change = (id: number, tool: string) => ({
  ...mockChange({ id, tool }),
  undone_at: null,
})

describe("buildTranscript", () => {
  it("shows prose only and puts change lines after the turn that made them", () => {
    const { turns, rest } = buildTranscript(
      [
        msg(1, "user", [{ type: "text", text: "chili 6" }]),
        msg(2, "assistant", [
          { type: "text", text: "On it." },
          { type: "tool_use", tool_use_id: "a", tool_use_name: "get_plan" },
          {
            type: "tool_use",
            tool_use_id: "b",
            tool_use_name: "set_session_portions",
          },
        ]),
        msg(3, "tool", [{ type: "tool_result", tool_result_id: "b" }]),
        msg(4, "assistant", [{ type: "text", text: "Done." }]),
      ],
      [change(5, "set_session_portions")]
    )
    expect(turns.map((t) => [t.message.id, t.text, t.changes.length])).toEqual([
      [1, "chili 6", 0],
      [2, "On it.", 1],
      [4, "Done.", 0],
    ])
    expect(rest).toEqual([])
  })

  it("skips failed calls and keeps unmatched changes for the end", () => {
    const { turns, rest } = buildTranscript(
      [
        msg(2, "assistant", [
          {
            type: "tool_use",
            tool_use_id: "b",
            tool_use_name: "remove_plate_component",
          },
        ]),
        msg(3, "user", [
          {
            type: "tool_result",
            tool_result_id: "b",
            tool_result_is_error: true,
          },
        ]),
      ],
      [change(9, "remove_plate_component")]
    )
    expect(turns).toEqual([])
    expect(rest.map((c) => c.id)).toEqual([9])
  })

  it("does not give an unlock preview the unlock change", () => {
    const { turns, rest } = buildTranscript(
      [
        msg(2, "assistant", [
          { type: "tool_use", tool_use_id: "p", tool_use_name: "unlock" },
        ]),
        msg(3, "tool", [
          {
            type: "tool_result",
            tool_result_id: "p",
            tool_result_content: { status: "needs_user_confirmation" },
          },
        ]),
        msg(4, "user", [{ type: "text", text: "Yes, unlock" }]),
        msg(5, "assistant", [
          { type: "tool_use", tool_use_id: "q", tool_use_name: "unlock" },
        ]),
      ],
      [change(9, "unlock")]
    )
    expect(turns.map((t) => [t.message.id, t.changes.length])).toEqual([
      [4, 0],
      [5, 1],
    ])
    expect(rest).toEqual([])
  })
})
