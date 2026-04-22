import "@testing-library/jest-dom/vitest"
import { afterEach, vi } from "vitest"
import { cleanup } from "@testing-library/react"

// Radix UI primitives (Select, Popover, etc.) call these APIs on pointer
// interactions. jsdom doesn't implement them, so we stub so userEvent-driven
// interactions don't throw.
if (typeof window !== "undefined") {
  window.HTMLElement.prototype.hasPointerCapture ??= vi.fn()
  window.HTMLElement.prototype.releasePointerCapture ??= vi.fn()
  window.HTMLElement.prototype.scrollIntoView ??= vi.fn()
}

// Radix Tooltip (and a few other primitives) observe element size via
// ResizeObserver; jsdom doesn't ship one.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver
}

afterEach(() => {
  cleanup()
})
