import { describe, expect, it } from "vitest"

import { BOTTOM_SLACK, isAtBottom } from "./use-stick-to-bottom"

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
