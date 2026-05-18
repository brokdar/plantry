import { fireEvent, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithRouter } from "@/test/render"

import { EmptyWeekCTA } from "./EmptyWeekCTA"

function noop() {}

// Bun's jsdom env exposes window.localStorage as a plain object — no Storage
// methods. Install an in-memory shim per test so the component's getItem /
// setItem calls round-trip and the dismissal assertions are meaningful.
function makeStorageShim() {
  const data = new Map<string, string>()
  return {
    getItem: (k: string) => (data.has(k) ? data.get(k)! : null),
    setItem: (k: string, v: string) => void data.set(k, String(v)),
    removeItem: (k: string) => void data.delete(k),
    clear: () => data.clear(),
    key: (i: number) => Array.from(data.keys())[i] ?? null,
    get length() {
      return data.size
    },
  }
}

let originalLocalStorage: Storage | undefined
beforeEach(() => {
  originalLocalStorage = window.localStorage
  Object.defineProperty(window, "localStorage", {
    value: makeStorageShim(),
    configurable: true,
    writable: true,
  })
})

afterEach(() => {
  Object.defineProperty(window, "localStorage", {
    value: originalLocalStorage,
    configurable: true,
    writable: true,
  })
})

describe("EmptyWeekCTA", () => {
  it("renders the empty-week CTA when not dismissed", async () => {
    renderWithRouter(
      <EmptyWeekCTA
        windowFrom="2026-04-27"
        aiEnabled={false}
        copying={false}
        onCopyLastWeek={noop}
        onCopyFromWeek={noop}
        onAiFill={noop}
      />
    )
    expect(await screen.findByTestId("empty-week-cta")).toBeTruthy()
    expect(screen.getByTestId("empty-week-copy")).toBeTruthy()
    expect(screen.getByTestId("empty-week-copy-from-week")).toBeTruthy()
    expect(screen.queryByTestId("empty-week-ai-fill")).toBeNull()
  })

  it("renders the AI fill action when aiEnabled is true", async () => {
    renderWithRouter(
      <EmptyWeekCTA
        windowFrom="2026-04-27"
        aiEnabled={true}
        copying={false}
        onCopyLastWeek={noop}
        onCopyFromWeek={noop}
        onAiFill={noop}
      />
    )
    expect(await screen.findByTestId("empty-week-ai-fill")).toBeTruthy()
  })

  it("dismissing persists to localStorage and hides the banner", async () => {
    const { rerender } = renderWithRouter(
      <EmptyWeekCTA
        windowFrom="2026-04-27"
        aiEnabled={false}
        copying={false}
        onCopyLastWeek={noop}
        onCopyFromWeek={noop}
        onAiFill={noop}
      />
    )
    const dismiss = await screen.findByTestId("empty-week-dismiss")
    fireEvent.click(dismiss)
    expect(screen.queryByTestId("empty-week-cta")).toBeNull()
    expect(
      window.localStorage.getItem("plantry.emptyWeekDismiss.2026-04-27")
    ).toBe("1")

    // Re-rendering the same window keeps it dismissed.
    rerender(
      <EmptyWeekCTA
        windowFrom="2026-04-27"
        aiEnabled={false}
        copying={false}
        onCopyLastWeek={noop}
        onCopyFromWeek={noop}
        onAiFill={noop}
      />
    )
    expect(screen.queryByTestId("empty-week-cta")).toBeNull()
  })

  it("a different window key shows the banner again", async () => {
    window.localStorage.setItem("plantry.emptyWeekDismiss.2026-04-27", "1")
    renderWithRouter(
      <EmptyWeekCTA
        windowFrom="2026-05-04"
        aiEnabled={false}
        copying={false}
        onCopyLastWeek={noop}
        onCopyFromWeek={noop}
        onAiFill={noop}
      />
    )
    expect(await screen.findByTestId("empty-week-cta")).toBeTruthy()
  })

  it("invokes onCopyLastWeek when the copy button is clicked", async () => {
    const onCopy = vi.fn()
    renderWithRouter(
      <EmptyWeekCTA
        windowFrom="2026-04-27"
        aiEnabled={false}
        copying={false}
        onCopyLastWeek={onCopy}
        onCopyFromWeek={noop}
        onAiFill={noop}
      />
    )
    fireEvent.click(await screen.findByTestId("empty-week-copy"))
    expect(onCopy).toHaveBeenCalledOnce()
  })
})
