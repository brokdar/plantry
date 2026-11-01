import { describe, expect, it } from "vitest"

import type { ConversationMessage } from "@/lib/domain/assistantEvents"
import { mockChange } from "@/test/fixtures"

import { buildTranscript, firstUserText } from "./transcript"

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
    expect(turns.map((t) => [t.message.id, t.changes.length])).toEqual([[2, 0]])
    expect(turns[0].failures).toEqual([
      { id: "b", tool: "remove_plate_component" },
    ])
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

  it("gives a regenerate change to the run that finished, not the failed one", () => {
    const { turns, rest } = buildTranscript(
      [
        msg(2, "assistant", [
          { type: "tool_use", tool_use_id: "r1", tool_use_name: "regenerate" },
        ]),
        msg(3, "tool", [
          {
            type: "tool_result",
            tool_result_id: "r1",
            tool_result_content: JSON.stringify({ status: "failed" }),
          },
        ]),
        msg(4, "assistant", [
          { type: "text", text: "That failed, trying again." },
          { type: "tool_use", tool_use_id: "r2", tool_use_name: "regenerate" },
        ]),
        msg(5, "tool", [
          {
            type: "tool_result",
            tool_result_id: "r2",
            tool_result_content: { status: "ok" },
          },
        ]),
      ],
      [change(9, "regenerate")]
    )
    expect(turns.map((t) => [t.message.id, t.changes.length])).toEqual([
      [2, 0],
      [4, 1],
    ])
    expect(turns[0].failures.map((f) => f.id)).toEqual(["r1"])
    expect(rest).toEqual([])
  })

  it("matches a change by its tool's kind suffix so later lines keep their turn", () => {
    const { turns, rest } = buildTranscript(
      [
        msg(2, "assistant", [
          { type: "tool_use", tool_use_id: "a", tool_use_name: "regenerate" },
        ]),
        msg(3, "tool", [
          {
            type: "tool_result",
            tool_result_id: "a",
            tool_result_content: { status: "ok" },
          },
        ]),
        msg(4, "assistant", [
          { type: "tool_use", tool_use_id: "b", tool_use_name: "unlock" },
        ]),
      ],
      [change(8, "regenerate_side"), change(9, "unlock")]
    )
    expect(
      turns.map((t) => [t.message.id, t.changes.map((c) => c.id)])
    ).toEqual([
      [2, [8]],
      [4, [9]],
    ])
    expect(rest).toEqual([])
  })

  it("reports a failed call once, per turn, as a failure and not a change", () => {
    const { turns } = buildTranscript(
      [
        msg(2, "assistant", [
          { type: "tool_use", tool_use_id: "r1", tool_use_name: "regenerate" },
        ]),
        msg(3, "tool", [
          {
            type: "tool_result",
            tool_result_id: "r1",
            tool_result_is_error: true,
            tool_result_content: "not found",
          },
        ]),
      ],
      []
    )
    expect(turns).toHaveLength(1)
    expect(turns[0].failures).toEqual([{ id: "r1", tool: "regenerate" }])
  })

  describe("change_id", () => {
    const call = (id: string, tool: string) => ({
      type: "tool_use",
      tool_use_id: id,
      tool_use_name: tool,
    })
    const result = (id: string, content: unknown, isError = false) => ({
      type: "tool_result",
      tool_result_id: id,
      tool_result_is_error: isError,
      tool_result_content: content,
    })

    it("gives a change to the call whose result names it, whatever the tool", () => {
      const { turns, rest } = buildTranscript(
        [
          msg(2, "assistant", [call("a", "set_availability")]),
          msg(3, "tool", [result("a", { change_id: 9 })]),
          msg(4, "assistant", [call("b", "set_availability")]),
          msg(5, "tool", [result("b", { change_id: 8 })]),
        ],
        [change(8, "set_availability"), change(9, "set_availability")]
      )
      expect(
        turns.map((t) => [t.message.id, t.changes.map((c) => c.id)])
      ).toEqual([
        [2, [9]],
        [4, [8]],
      ])
      expect(rest).toEqual([])
    })

    it("reads change_id from JSON text too", () => {
      const { turns } = buildTranscript(
        [
          msg(2, "assistant", [call("a", "x")]),
          msg(3, "tool", [result("a", JSON.stringify({ change_id: 4 }))]),
        ],
        [change(4, "unrelated_tool")]
      )
      expect(turns[0].changes.map((c) => c.id)).toEqual([4])
    })

    it("shows the change of a failed call next to its failure line", () => {
      const { turns, rest } = buildTranscript(
        [
          msg(2, "assistant", [call("r", "regenerate")]),
          msg(3, "tool", [result("r", { change_id: 7 }, true)]),
        ],
        [change(7, "regenerate_side")]
      )
      expect(turns[0].failures).toEqual([{ id: "r", tool: "regenerate" }])
      expect(turns[0].changes.map((c) => c.id)).toEqual([7])
      expect(rest).toEqual([])
    })

    it("does not let name matching take a change another call names", () => {
      const { turns, rest } = buildTranscript(
        [
          msg(2, "assistant", [call("old", "unlock"), call("new", "unlock")]),
          msg(3, "tool", [result("new", { change_id: 3 })]),
        ],
        [change(3, "unlock")]
      )
      expect(turns[0].changes.map((c) => c.id)).toEqual([3])
      expect(rest).toEqual([])
    })

    it("keeps calls apart when a tool call id is reused in a later turn", () => {
      const { turns, rest } = buildTranscript(
        [
          msg(2, "assistant", [call("tu_1", "set_session_portions")]),
          msg(3, "tool", [result("tu_1", { change_id: 5 })]),
          msg(4, "assistant", [call("tu_1", "set_session_portions")]),
          msg(5, "tool", [result("tu_1", { change_id: 6 })]),
        ],
        [change(5, "set_session_portions"), change(6, "set_session_portions")]
      )
      expect(
        turns.map((t) => [t.message.id, t.changes.map((c) => c.id)])
      ).toEqual([
        [2, [5]],
        [4, [6]],
      ])
      expect(rest).toEqual([])
    })

    it("keeps a change whose call is missing for the end", () => {
      const { rest } = buildTranscript(
        [msg(3, "tool", [result("gone", { change_id: 3 })])],
        [change(3, "unlock")]
      )
      expect(rest.map((c) => c.id)).toEqual([3])
    })

    it("a call whose change was not recorded takes no other change", () => {
      const { turns, rest } = buildTranscript(
        [
          msg(2, "assistant", [call("a", "unlock")]),
          msg(3, "tool", [result("a", { warning: "change_not_recorded" })]),
        ],
        [change(9, "unlock")]
      )
      expect(turns).toEqual([])
      expect(rest.map((c) => c.id)).toEqual([9])
    })
  })

  describe("pendingUnlock", () => {
    const preview = [
      msg(2, "assistant", [
        { type: "tool_use", tool_use_id: "p", tool_use_name: "unlock" },
      ]),
      msg(3, "tool", [
        {
          type: "tool_result",
          tool_result_id: "p",
          tool_result_content: JSON.stringify({
            status: "needs_user_confirmation",
            rows: [{ date: "2026-11-02", slot_id: 3, names: ["Chili"] }],
          }),
        },
      ]),
    ]

    it("is the stored preview while no user turn followed", () => {
      expect(buildTranscript(preview, []).pendingUnlock).toEqual({
        rows: [{ date: "2026-11-02", slot_id: 3, names: ["Chili"] }],
      })
    })

    it("is null once the user answered", () => {
      const answered = [
        ...preview,
        msg(4, "user", [{ type: "text", text: "Yes, unlock" }]),
      ]
      expect(buildTranscript(answered, []).pendingUnlock).toBeNull()
    })

    it("is not a failure", () => {
      const { turns } = buildTranscript(preview, [])
      expect(turns.every((t) => t.failures.length === 0)).toBe(true)
    })
  })
})

describe("firstUserText", () => {
  it("is the first user message, one line, cut with an ellipsis", () => {
    const long = "Make the chili 6 portions and freeze 2 of them please"
    expect(
      firstUserText([
        msg(1, "user", [{ type: "text", text: `${long}\nthanks` }]),
        msg(2, "user", [{ type: "text", text: "later" }]),
      ])
    ).toBe(`${long.slice(0, 47).trimEnd()}…`)
    expect(firstUserText([msg(1, "assistant", [])])).toBe("")
    expect(
      firstUserText([msg(1, "user", [{ type: "text", text: " hi " }])])
    ).toBe("hi")
  })
})
