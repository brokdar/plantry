import "@testing-library/jest-dom/vitest"
import { afterEach, vi } from "vitest"
import { cleanup } from "@testing-library/react"
import { initUnitsVocabulary } from "@/lib/domain/units"

// Seed the canonical unit vocabulary so tests that exercise unit-aware
// components (QuantityUnitInput, UnitSelect, etc.) work without a real
// network call. Mirrors the data served by GET /api/units.
initUnitsVocabulary([
  { id: "g", group: "mass", grams: 1, approximate: false },
  { id: "kg", group: "mass", grams: 1000, approximate: false },
  { id: "mg", group: "mass", grams: 0.001, approximate: false },
  { id: "oz", group: "mass", grams: 28.3495, approximate: false },
  { id: "lb", group: "mass", grams: 453.592, approximate: false },
  { id: "ml", group: "volume", grams: 1, approximate: true },
  { id: "l", group: "volume", grams: 1000, approximate: true },
  { id: "cl", group: "volume", grams: 10, approximate: true },
  { id: "dl", group: "volume", grams: 100, approximate: true },
  { id: "tbsp", group: "volume", grams: 15, approximate: true },
  { id: "tsp", group: "volume", grams: 5, approximate: true },
  { id: "cup", group: "volume", grams: 240, approximate: true },
  { id: "floz", group: "volume", grams: 29.5735, approximate: true },
  { id: "piece", group: "count" },
  { id: "clove", group: "count" },
  { id: "slice", group: "count" },
  { id: "bunch", group: "count" },
  { id: "pinch", group: "count" },
  { id: "stick", group: "count" },
  { id: "can", group: "count" },
  { id: "jar", group: "count" },
  { id: "packet", group: "count" },
  { id: "serving", group: "count" },
])

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
