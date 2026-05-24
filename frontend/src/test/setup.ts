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

// Node.js v26 defines a non-functional localStorage stub (requires
// --localstorage-file). Replace it with a real in-memory implementation
// so tests that access localStorage directly don't throw.
if (typeof globalThis.localStorage?.getItem !== "function") {
  const _store = new Map<string, string>()
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    writable: true,
    value: {
      getItem: (k: string) => _store.get(k) ?? null,
      setItem: (k: string, v: string) => {
        _store.set(k, String(v))
      },
      removeItem: (k: string) => {
        _store.delete(k)
      },
      clear: () => {
        _store.clear()
      },
      key: (i: number) => [..._store.keys()][i] ?? null,
      get length() {
        return _store.size
      },
    },
  })
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
