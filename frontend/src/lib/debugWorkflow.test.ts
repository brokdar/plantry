import { afterEach, describe, expect, test } from "vitest"
import { act, renderHook } from "@testing-library/react"

import { getDebugWorkflow, useDebugWorkflow } from "./debugWorkflow"

afterEach(() => {
  // Reset via the public API so this test does not depend on localStorage
  // being directly mutable in the test environment.
  const { result } = renderHook(() => useDebugWorkflow())
  act(() => result.current[1](false))
})

describe("debugWorkflow", () => {
  test("defaults to false", () => {
    expect(getDebugWorkflow()).toBe(false)
    const { result } = renderHook(() => useDebugWorkflow())
    expect(result.current[0]).toBe(false)
  })

  test("setter flips the flag and re-renders the hook", () => {
    const { result } = renderHook(() => useDebugWorkflow())
    act(() => result.current[1](true))
    expect(result.current[0]).toBe(true)
    expect(getDebugWorkflow()).toBe(true)
  })

  test("multiple hook instances stay in sync", () => {
    const a = renderHook(() => useDebugWorkflow())
    const b = renderHook(() => useDebugWorkflow())
    act(() => a.result.current[1](true))
    expect(a.result.current[0]).toBe(true)
    expect(b.result.current[0]).toBe(true)
  })
})
