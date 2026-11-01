import { beforeEach, describe, expect, it } from "vitest"

import { mockChange } from "@/test/fixtures"

import { assistantStream } from "./assistant-stream"

describe("assistantStream", () => {
  beforeEach(() => assistantStream.clear())

  it("keeps change line states when a new turn starts", () => {
    assistantStream.setLineState(5, "stale")
    assistantStream.setDraft("next")
    assistantStream.appendText("hi")
    assistantStream.reset()
    expect(assistantStream.get().lineStates).toEqual({ 5: "stale" })
    expect(assistantStream.get().text).toBe("")
    expect(assistantStream.get().draft).toBe("next")
  })

  it("drops a repeated recorded change but keeps each unrecorded one", () => {
    assistantStream.addChange(mockChange({ id: 5 }))
    assistantStream.addChange(mockChange({ id: 5 }))
    const lost = mockChange({ id: 0, recorded: false, undoable: false })
    assistantStream.addChange(lost)
    assistantStream.addChange(lost)
    expect(assistantStream.get().changes.map((c) => c.id)).toEqual([5, 0, 0])
  })
})
