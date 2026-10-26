import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { renderHook, waitFor } from "@testing-library/react"

import { queryClient } from "@/lib/query-client"
import { mockChickenCurry } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

import { useCreateFood, useCreateVariant, useDeleteFood } from "./foods"
import { setupKeys } from "./keys"

vi.mock("@/lib/api/foods", () => ({
  createFood: vi.fn(),
  deleteFood: vi.fn(),
  createVariant: vi.fn(),
}))

import { createFood, createVariant, deleteFood } from "@/lib/api/foods"

// The Generate gate and the guided Recipes state read the recipe count from
// GET /api/setup, so every mutation that changes it must refresh that query.
describe("recipe-count mutations invalidate the setup status", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("useCreateFood", async () => {
    vi.mocked(createFood).mockResolvedValue(mockChickenCurry)
    const { result } = renderHook(() => useCreateFood(), {
      wrapper: createHookWrapper(),
    })
    result.current.mutate({ kind: "composed", name: "Curry" } as never)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: setupKeys.status(),
    })
  })

  it("useDeleteFood", async () => {
    vi.mocked(deleteFood).mockResolvedValue(undefined)
    const { result } = renderHook(() => useDeleteFood(), {
      wrapper: createHookWrapper(),
    })
    result.current.mutate(mockChickenCurry.id)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: setupKeys.status(),
    })
  })

  it("useCreateVariant", async () => {
    vi.mocked(createVariant).mockResolvedValue(mockChickenCurry)
    const { result } = renderHook(() => useCreateVariant(), {
      wrapper: createHookWrapper(),
    })
    result.current.mutate(mockChickenCurry.id)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: setupKeys.status(),
    })
  })
})
