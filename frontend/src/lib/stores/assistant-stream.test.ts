import { beforeEach, describe, expect, it } from "vitest"

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
})
