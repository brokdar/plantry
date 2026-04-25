import { describe, it, expect, vi, beforeEach } from "vitest"
import { renderHook, waitFor } from "@testing-library/react"
import { useVariants, useCreateVariant } from "@/lib/queries/foods"
import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/foods", () => ({
  listVariants: vi.fn(),
  createVariant: vi.fn(),
}))

import { listVariants, createVariant } from "@/lib/api/foods"
import { mockTofuCurryVariant } from "@/test/fixtures"

describe("useVariants", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("fetches variants for a food", async () => {
    vi.mocked(listVariants).mockResolvedValue({
      items: [mockTofuCurryVariant],
    })

    const { result } = renderHook(() => useVariants(10), {
      wrapper: createHookWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(listVariants).toHaveBeenCalledWith(10)
    expect(result.current.data?.items).toHaveLength(1)
    expect(result.current.data?.items[0].name).toBe("Tofu Curry")
  })

  it("does not fetch when id is 0", async () => {
    const { result } = renderHook(() => useVariants(0), {
      wrapper: createHookWrapper(),
    })

    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"))
    expect(listVariants).not.toHaveBeenCalled()
  })
})

describe("useCreateVariant", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("calls createVariant API and invalidates queries", async () => {
    const newVariant = { ...mockTofuCurryVariant, id: 99 }
    vi.mocked(createVariant).mockResolvedValue(newVariant)

    const { result } = renderHook(() => useCreateVariant(), {
      wrapper: createHookWrapper(),
    })

    result.current.mutate(10)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(createVariant).toHaveBeenCalledWith(10)
  })
})
