import { describe, expect, it } from "vitest"

import { mockSettingItem } from "@/test/fixtures"

import { discoveryAvailable } from "./available"

describe("discoveryAvailable", () => {
  it("is true with a configured provider", () => {
    expect(
      discoveryAvailable({
        "ai.provider": mockSettingItem("ai.provider", "anthropic", "db"),
      })
    ).toBe(true)
  })
  it("is false without one, or while settings load", () => {
    expect(
      discoveryAvailable({
        "ai.provider": mockSettingItem("ai.provider", ""),
      })
    ).toBe(false)
    expect(discoveryAvailable(undefined)).toBe(false)
  })
})
