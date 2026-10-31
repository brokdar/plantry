import { beforeEach, describe, expect, it, vi } from "vitest"

import { invalidatedBy } from "@/test/render"

vi.mock("@/lib/api/settings", () => ({
  listSettings: vi.fn(),
  setSetting: vi.fn(),
  clearSetting: vi.fn(),
  getSystemInfo: vi.fn(),
}))

import { clearSetting, setSetting } from "@/lib/api/settings"

import { insightKeys, runKeys } from "./keys"
import { useClearSetting, useSetSetting } from "./settings"

describe("settings mutations", () => {
  beforeEach(() => {
    vi.mocked(setSetting).mockResolvedValue(undefined)
    vi.mocked(clearSetting).mockResolvedValue(undefined)
  })

  it("a price override re-prices runs and insights (F7-AC3)", async () => {
    const keys = await invalidatedBy(useSetSetting, {
      key: "ai.price_overrides",
      value: "{}",
    })
    expect(keys).toContainEqual(runKeys.all)
    expect(keys).toContainEqual(insightKeys.all)

    const cleared = await invalidatedBy(useClearSetting, "ai.price_overrides")
    expect(cleared).toContainEqual(runKeys.all)
    expect(cleared).toContainEqual(insightKeys.all)
  })

  it("other settings leave the run log alone", async () => {
    const keys = await invalidatedBy(useSetSetting, {
      key: "ai.rate_limit_per_min",
      value: "5",
    })
    expect(keys).not.toContainEqual(runKeys.all)
    expect(keys).not.toContainEqual(insightKeys.all)
  })
})
