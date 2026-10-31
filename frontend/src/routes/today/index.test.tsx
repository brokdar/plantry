import { isRedirect } from "@tanstack/react-router"
import { beforeEach, describe, expect, it, vi } from "vitest"

vi.mock("@/lib/api/setup", () => ({
  getSetupStatus: vi.fn(),
  completeSetup: vi.fn(),
}))

import { getSetupStatus } from "@/lib/api/setup"
import { queryClient } from "@/lib/query-client"

import { Route } from "./index"

async function runBeforeLoad(): Promise<unknown> {
  const beforeLoad = Route.options.beforeLoad as () => Promise<void>
  try {
    await beforeLoad()
    return undefined
  } catch (err) {
    return err
  }
}

describe("/today setup guard (F5-AC5)", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryClient.clear()
  })

  it("redirects to /setup while setup is not completed", async () => {
    vi.mocked(getSetupStatus).mockResolvedValue({
      completed: false,
      recipe_count: 0,
      recipe_target: 10,
    })
    const thrown = await runBeforeLoad()
    expect(isRedirect(thrown)).toBe(true)
    expect((thrown as { options: { to: string } }).options.to).toBe("/setup")
  })

  it("opens once setup is completed", async () => {
    vi.mocked(getSetupStatus).mockResolvedValue({
      completed: true,
      recipe_count: 3,
      recipe_target: 10,
    })
    expect(await runBeforeLoad()).toBeUndefined()
  })

  it("keeps only a valid date in the search", () => {
    const validate = Route.options.validateSearch as (
      s: Record<string, unknown>
    ) => { date?: string }
    expect(validate({ date: "2026-11-02" })).toEqual({ date: "2026-11-02" })
    expect(validate({ date: "monday" })).toEqual({ date: undefined })
  })
})
