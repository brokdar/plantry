import type { TFunction } from "i18next"
import { beforeEach, describe, expect, test, vi } from "vitest"

import type { ApplyResult, ApplyResultCell } from "@/lib/api/presets"
import type { SnapshotPlate } from "@/lib/api/plates"

vi.mock("@/lib/toast", () => ({
  toast: {
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
  },
}))

import { toast } from "@/lib/toast"

import { showPresetApplyToasts } from "./preset-apply-toast"

const mockedToast = toast as unknown as {
  success: ReturnType<typeof vi.fn>
  warning: ReturnType<typeof vi.fn>
  error: ReturnType<typeof vi.fn>
}

// A simple TFunction stand-in: returns the key suffixed with any count for
// determinism in assertions. Cast to TFunction since we don't exercise typing.
const t = ((key: string, opts?: { count?: number }) =>
  opts?.count !== undefined
    ? `${key}:${opts.count}`
    : key) as unknown as TFunction

function skip(slotID: number): ApplyResultCell {
  return { date: "2026-05-20", slot_id: slotID }
}

const replacedPlate: SnapshotPlate = {
  date: "2026-05-20",
  slot_id: 1,
  eaters: 2,
  note: null,
  components: [],
}

function result(overrides: Partial<ApplyResult> = {}): ApplyResult {
  return {
    created_plate_ids: [],
    skipped_occupied: [],
    skipped_no_slot: [],
    skipped_unavailable: [],
    removed: { plates: [], sessions: [] },
    created_overrides: [],
    ...overrides,
  }
}

describe("showPresetApplyToasts", () => {
  beforeEach(() => {
    mockedToast.success.mockReset()
    mockedToast.warning.mockReset()
    mockedToast.error.mockReset()
  })

  test("created_only: success without undo action", () => {
    const onUndo = vi.fn()
    showPresetApplyToasts(result({ created_plate_ids: [1] }), t, onUndo)

    expect(mockedToast.success).toHaveBeenCalledTimes(1)
    const [, options] = mockedToast.success.mock.calls[0]
    // No second arg or no action means no undo.
    expect(options).toBeUndefined()
    expect(mockedToast.warning).not.toHaveBeenCalled()
    expect(onUndo).not.toHaveBeenCalled()
  })

  test("replaced_carries_undo: action invokes onUndo with the result", () => {
    const onUndo = vi.fn()
    const applied = result({
      created_plate_ids: [42],
      removed: { plates: [replacedPlate], sessions: [] },
    })
    showPresetApplyToasts(applied, t, onUndo)

    expect(mockedToast.success).toHaveBeenCalledTimes(1)
    const [, options] = mockedToast.success.mock.calls[0]
    expect(options).toBeDefined()
    expect(options.action).toBeDefined()
    expect(typeof options.action.onClick).toBe("function")

    options.action.onClick()
    expect(onUndo).toHaveBeenCalledTimes(1)
    expect(onUndo).toHaveBeenCalledWith(applied)
    expect(mockedToast.success.mock.calls[0][0]).toBe(
      "preset.apply.result_replaced:1"
    )
  })

  test("all_skipped_occupied: warning only, no success", () => {
    const onUndo = vi.fn()
    showPresetApplyToasts(result({ skipped_occupied: [skip(1)] }), t, onUndo)

    expect(mockedToast.warning).toHaveBeenCalledTimes(1)
    expect(mockedToast.success).not.toHaveBeenCalled()
  })

  test("mixed_created_and_skipped: success then warning, order matters", () => {
    const onUndo = vi.fn()
    showPresetApplyToasts(
      result({ created_plate_ids: [1], skipped_occupied: [skip(2)] }),
      t,
      onUndo
    )

    expect(mockedToast.success).toHaveBeenCalledTimes(1)
    expect(mockedToast.warning).toHaveBeenCalledTimes(1)

    const successOrder = mockedToast.success.mock.invocationCallOrder[0]
    const warningOrder = mockedToast.warning.mock.invocationCallOrder[0]
    expect(successOrder).toBeLessThan(warningOrder)
  })

  test("skipped_unavailable: reports No-plate cells after the success", () => {
    showPresetApplyToasts(
      result({
        created_plate_ids: [1],
        skipped_unavailable: [skip(2), skip(3)],
      }),
      t,
      vi.fn()
    )

    expect(mockedToast.success).toHaveBeenCalledTimes(1)
    expect(mockedToast.warning).toHaveBeenCalledWith(
      "preset.apply.result_skipped_unavailable:2"
    )
  })

  test("only_skipped_unavailable: warning only, no success", () => {
    showPresetApplyToasts(
      result({ skipped_unavailable: [skip(2)] }),
      t,
      vi.fn()
    )

    expect(mockedToast.success).not.toHaveBeenCalled()
    expect(mockedToast.warning).toHaveBeenCalledWith(
      "preset.apply.result_skipped_unavailable:1"
    )
  })

  test("totally_empty_result: neither success nor warning called", () => {
    const onUndo = vi.fn()
    showPresetApplyToasts(result(), t, onUndo)

    expect(mockedToast.success).not.toHaveBeenCalled()
    expect(mockedToast.warning).not.toHaveBeenCalled()
  })
})
