import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockProfile } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/profile", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/profile")>()),
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({ toastError: vi.fn() }))

import { ApiError } from "@/lib/api/client"
import { updateProfile, type Profile } from "@/lib/api/profile"
import { toastError } from "@/lib/toast"

import { profileKeys } from "./keys"
import { usePatchProfile } from "./profile"

function setup() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  qc.setQueryData(profileKeys.detail, mockProfile)
  return { qc, wrapper: createHookWrapper(qc) }
}

describe("usePatchProfile", () => {
  beforeEach(() => vi.clearAllMocks())

  it("sends the full profile with the patch on top", async () => {
    vi.mocked(updateProfile).mockImplementation(
      async (input) => ({ ...mockProfile, ...input }) as Profile
    )
    const { wrapper } = setup()
    const { result } = renderHook(() => usePatchProfile(), { wrapper })
    await act(() => result.current.mutateAsync({ diet_type: "vegan" }))
    expect(updateProfile).toHaveBeenCalledWith({
      kcal_target: null,
      protein_pct: null,
      fat_pct: null,
      carbs_pct: null,
      dietary_restrictions: [],
      preferences: {},
      system_prompt: null,
      locale: "en",
      diet_type: "vegan",
      meat_days_per_week_max: null,
      household_portions: 1,
    })
  })

  it("runs quick patches one after another, each carrying the earlier ones", async () => {
    let release!: () => void
    vi.mocked(updateProfile)
      .mockImplementationOnce(
        (input) =>
          new Promise((resolve) => {
            release = () => resolve({ ...mockProfile, ...input } as Profile)
          })
      )
      .mockImplementation(
        async (input) => ({ ...mockProfile, ...input }) as Profile
      )
    const { qc, wrapper } = setup()
    const { result } = renderHook(() => usePatchProfile(), { wrapper })

    act(() => {
      result.current.mutate({ diet_type: "vegan" })
      result.current.mutate({ household_portions: 2 })
    })
    // Both show at once in the cache.
    await waitFor(() =>
      expect(qc.getQueryData<Profile>(profileKeys.detail)).toMatchObject({
        diet_type: "vegan",
        household_portions: 2,
      })
    )
    expect(updateProfile).toHaveBeenCalledTimes(1)

    act(() => release())
    await waitFor(() => expect(updateProfile).toHaveBeenCalledTimes(2))
    expect(vi.mocked(updateProfile).mock.calls[1][0]).toMatchObject({
      diet_type: "vegan",
      household_portions: 2,
    })
  })

  it("toasts and refetches on failure", async () => {
    vi.mocked(updateProfile).mockRejectedValue(
      new ApiError(400, "error.profile.invalid_household_portions")
    )
    const { qc, wrapper } = setup()
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    const { result } = renderHook(() => usePatchProfile(), { wrapper })
    act(() => result.current.mutate({ household_portions: 50 }))
    await waitFor(() => expect(toastError).toHaveBeenCalled())
    expect(invalidate).toHaveBeenCalledWith({ queryKey: profileKeys.detail })
  })
})
