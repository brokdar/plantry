import { describe, it, expect, vi, beforeEach } from "vitest"
import { renderHook, waitFor } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { useVariants, useCreateVariant } from "@/lib/queries/foods"
import type { ReactNode } from "react"

vi.mock("@/lib/api/foods", () => ({
  listVariants: vi.fn(),
  createVariant: vi.fn(),
  listComponents: vi.fn(),
  getComponent: vi.fn(),
  getComponentNutrition: vi.fn(),
  deleteComponent: vi.fn(),
  createComponent: vi.fn(),
  updateComponent: vi.fn(),
}))

import { listVariants, createVariant } from "@/lib/api/foods"
import { mockTofuCurryVariant } from "@/test/fixtures"

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

describe("useVariants", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("fetches variants for a component", async () => {
    vi.mocked(listVariants).mockResolvedValue({
      items: [mockTofuCurryVariant],
    })

    const { result } = renderHook(() => useVariants(10), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(listVariants).toHaveBeenCalledWith(10)
    expect(result.current.data?.items).toHaveLength(1)
    expect(result.current.data?.items[0].name).toBe("Tofu Curry")
  })

  it("does not fetch when id is 0", async () => {
    const { result } = renderHook(() => useVariants(0), {
      wrapper: createWrapper(),
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
      wrapper: createWrapper(),
    })

    result.current.mutate(10)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(createVariant).toHaveBeenCalledWith(10)
  })
})
