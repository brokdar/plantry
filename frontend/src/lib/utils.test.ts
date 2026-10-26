import { describe, expect, it } from "vitest"

import { cn } from "./utils"

describe("cn", () => {
  it("keeps a type-scale token next to a colour token", () => {
    expect(cn("text-small text-on-accent")).toBe("text-small text-on-accent")
    expect(cn("text-on-accent", "text-body")).toBe("text-on-accent text-body")
  })

  it("lets the last type-scale token win", () => {
    expect(cn("text-small", "text-body")).toBe("text-body")
    expect(cn("text-h1", "text-caption")).toBe("text-caption")
    expect(cn("text-sm", "text-h2")).toBe("text-h2")
  })

  it("still lets the last colour win", () => {
    expect(cn("text-small text-ink", "text-on-accent")).toBe(
      "text-small text-on-accent"
    )
  })
})
