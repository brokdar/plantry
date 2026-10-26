import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/setup", () => ({
  getSetupStatus: vi.fn(),
  completeSetup: vi.fn(),
}))

vi.mock("@/lib/toast", () => ({ toastError: vi.fn() }))

import { ApiError } from "@/lib/api/client"
import { completeSetup, getSetupStatus } from "@/lib/api/setup"
import { toastError } from "@/lib/toast"

import { setupKeys, useCompleteSetup, useSetupStatus } from "./setup"

const incomplete = { completed: false, recipe_count: 0, recipe_target: 10 }

function setup() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const invalidate = vi.spyOn(qc, "invalidateQueries")
  return { qc, invalidate, wrapper: createHookWrapper(qc) }
}

describe("setup queries", () => {
  beforeEach(() => vi.clearAllMocks())

  it("useSetupStatus reads GET /api/setup under setupKeys.status()", async () => {
    vi.mocked(getSetupStatus).mockResolvedValue(incomplete)
    const { qc, wrapper } = setup()
    const { result } = renderHook(() => useSetupStatus(), { wrapper })
    await waitFor(() => expect(result.current.data).toEqual(incomplete))
    expect(qc.getQueryData(setupKeys.status())).toEqual(incomplete)
  })

  it("useCompleteSetup invalidates the setup status", async () => {
    vi.mocked(completeSetup).mockResolvedValue({
      ...incomplete,
      completed: true,
    })
    const { invalidate, wrapper } = setup()
    const { result } = renderHook(() => useCompleteSetup(), { wrapper })
    await act(() => result.current.mutateAsync())
    expect(invalidate).toHaveBeenCalledWith({ queryKey: setupKeys.status() })
  })

  it("useCompleteSetup toasts on failure", async () => {
    const err = new ApiError(500, "error.server")
    vi.mocked(completeSetup).mockRejectedValue(err)
    const { wrapper } = setup()
    const { result } = renderHook(() => useCompleteSetup(), { wrapper })
    act(() => result.current.mutate())
    await waitFor(() => expect(toastError).toHaveBeenCalled())
    expect(vi.mocked(toastError).mock.calls[0][0]).toBe(err)
  })
})
