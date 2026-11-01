import { renderHook } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import {
  BOTTOM_SLACK,
  isAtBottom,
  useStickToBottom,
} from "./use-stick-to-bottom"

describe("isAtBottom", () => {
  const el = (scrollTop: number) => ({
    scrollHeight: 1000,
    clientHeight: 400,
    scrollTop,
  })

  it("is true at the end and within the slack", () => {
    expect(isAtBottom(el(600))).toBe(true)
    expect(isAtBottom(el(600 - BOTTOM_SLACK))).toBe(true)
  })

  it("is false once the reader scrolled up", () => {
    expect(isAtBottom(el(600 - BOTTOM_SLACK - 1))).toBe(false)
    expect(isAtBottom(el(0))).toBe(false)
  })
})

describe("useStickToBottom", () => {
  // A plain object stands in for the scroll container: the hook only reads
  // scrollHeight, scrollTop and clientHeight and writes scrollTop.
  function setup() {
    const el = { scrollHeight: 1000, clientHeight: 400, scrollTop: 600 }
    const ref = { current: el as unknown as HTMLElement }
    const hook = renderHook(
      ({ content, reset }) => useStickToBottom(ref, content, reset),
      { initialProps: { content: 0, reset: "a" } }
    )
    /** The user scrolls to `top`: the container's handler runs. */
    const scrollTo = (top: number) => {
      el.scrollTop = top
      hook.result.current()
    }
    /** New content makes the container taller. */
    const grow = (height: number, content: number, reset = "a") => {
      el.scrollHeight = height
      hook.rerender({ content, reset })
    }
    return { el, scrollTo, grow }
  }

  it("follows new content while at the bottom", () => {
    const { el, grow } = setup()
    grow(1200, 1)
    expect(el.scrollTop).toBe(1200)
    grow(1500, 2)
    expect(el.scrollTop).toBe(1500)
  })

  it("stays put after the user scrolls up", () => {
    const { el, scrollTo, grow } = setup()
    scrollTo(100)
    grow(1400, 1)
    expect(el.scrollTop).toBe(100)
  })

  it("resumes once the user scrolls back to the bottom", () => {
    const { el, scrollTo, grow } = setup()
    scrollTo(100)
    grow(1400, 1)
    scrollTo(1400 - el.clientHeight)
    grow(1600, 2)
    expect(el.scrollTop).toBe(1600)
  })

  it("follows again for a new conversation", () => {
    const { el, scrollTo, grow } = setup()
    scrollTo(100)
    grow(1400, 1)
    expect(el.scrollTop).toBe(100)
    grow(800, 2, "b")
    expect(el.scrollTop).toBe(800)
  })
})
