import { beforeEach, describe, expect, it, vi } from "vitest"

import { invalidatedBy } from "@/test/render"

vi.mock("@/lib/api/feedback", () => ({
  putFeedback: vi.fn(),
  deleteFeedback: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({ toast: vi.fn(), toastError: vi.fn() }))

import { deleteFeedback, putFeedback } from "@/lib/api/feedback"

import { useClearFeedback, useRecordFeedback } from "./feedback"
import { foodKeys, planKeys, shoppingKeys, todayKeys } from "./keys"

// F1-AC2: feedback refreshes the plan views and, because `not_eaten` changes
// the derived eating history (D16), the foods.
const FEEDBACK_VIEWS = [
  planKeys.all,
  todayKeys.all,
  shoppingKeys.all,
  foodKeys.all,
]

beforeEach(() => vi.clearAllMocks())

describe("feedback mutations", () => {
  it("useRecordFeedback", async () => {
    vi.mocked(putFeedback).mockResolvedValue({
      plate_id: 1,
      status: "not_eaten",
      rated_at: "2026-11-02T20:00:00Z",
    })
    const input = { status: "not_eaten" as const, note: null }
    expect(
      await invalidatedBy(() => useRecordFeedback(), { plateId: 1, input })
    ).toEqual(FEEDBACK_VIEWS)
    expect(putFeedback).toHaveBeenCalledWith(1, input)
  })

  it("useClearFeedback", async () => {
    vi.mocked(deleteFeedback).mockResolvedValue(undefined)
    expect(await invalidatedBy(() => useClearFeedback(), 1)).toEqual(
      FEEDBACK_VIEWS
    )
    expect(deleteFeedback).toHaveBeenCalledWith(1)
  })
})
