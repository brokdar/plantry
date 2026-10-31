import { afterEach, describe, expect, it, vi } from "vitest"

import {
  cellShortcut,
  handleGridArrowKey,
  isAcceptShortcut,
  isCheatsheetShortcut,
  readCellPos,
} from "./keynav"

function makeGrid(rows: number, cols: number): HTMLDivElement {
  const root = document.createElement("div")
  for (let r = 0; r < rows; r++) {
    for (let c = 0; c < cols; c++) {
      const cell = document.createElement("div")
      cell.setAttribute("data-cell-pos", `${r}-${c}`)
      const btn = document.createElement("button")
      btn.setAttribute("data-cell-focus", "")
      btn.textContent = `${r}-${c}`
      cell.appendChild(btn)
      root.appendChild(cell)
    }
  }
  document.body.appendChild(root)
  return root
}

function clearDom() {
  while (document.body.firstChild) {
    document.body.removeChild(document.body.firstChild)
  }
}

interface FakeKeyEvent {
  key: string
  metaKey?: boolean
  ctrlKey?: boolean
  altKey?: boolean
  target: Element
  currentTarget: Element
  preventDefault: () => void
}

function makeKeyEvent(
  key: string,
  target: Element,
  currentTarget: Element
): FakeKeyEvent {
  return {
    key,
    target,
    currentTarget,
    preventDefault: vi.fn(),
  }
}

afterEach(() => {
  clearDom()
})

describe("readCellPos", () => {
  it("reads pos off the closest ancestor", () => {
    const root = makeGrid(2, 3)
    const btn = root.querySelector(
      '[data-cell-pos="1-2"] button'
    ) as HTMLButtonElement
    expect(readCellPos(btn)).toEqual({ row: 1, col: 2 })
  })

  it("returns null when no pos ancestor", () => {
    const stray = document.createElement("button")
    expect(readCellPos(stray)).toBeNull()
  })
})

describe("handleGridArrowKey", () => {
  function focusedPos(): { row: number; col: number } | null {
    const active = document.activeElement
    if (!active) return null
    return readCellPos(active)
  }

  it("ArrowRight moves one column", () => {
    const root = makeGrid(3, 7)
    const start = root.querySelector(
      '[data-cell-pos="1-2"] button'
    ) as HTMLButtonElement
    start.focus()
    const e = makeKeyEvent("ArrowRight", start, root)
    handleGridArrowKey(e as never, { rows: 3, cols: 7 })
    expect(focusedPos()).toEqual({ row: 1, col: 3 })
    expect(e.preventDefault).toHaveBeenCalled()
  })

  it("F3-AC4: ArrowRight at a row end wraps to the next row", () => {
    const root = makeGrid(3, 7)
    const start = root.querySelector(
      '[data-cell-pos="0-6"] button'
    ) as HTMLButtonElement
    start.focus()
    handleGridArrowKey(makeKeyEvent("ArrowRight", start, root) as never, {
      rows: 3,
      cols: 7,
    })
    expect(focusedPos()).toEqual({ row: 1, col: 0 })
  })

  it("F3-AC4: ArrowLeft at a row start wraps to the previous row", () => {
    const root = makeGrid(3, 7)
    const start = root.querySelector(
      '[data-cell-pos="1-0"] button'
    ) as HTMLButtonElement
    start.focus()
    handleGridArrowKey(makeKeyEvent("ArrowLeft", start, root) as never, {
      rows: 3,
      cols: 7,
    })
    expect(focusedPos()).toEqual({ row: 0, col: 6 })
  })

  it("F3-AC4: wraps from the last cell to the first and back", () => {
    const root = makeGrid(3, 7)
    const last = root.querySelector(
      '[data-cell-pos="2-6"] button'
    ) as HTMLButtonElement
    last.focus()
    handleGridArrowKey(makeKeyEvent("ArrowRight", last, root) as never, {
      rows: 3,
      cols: 7,
    })
    expect(focusedPos()).toEqual({ row: 0, col: 0 })
    const first = document.activeElement as HTMLElement
    handleGridArrowKey(makeKeyEvent("ArrowLeft", first, root) as never, {
      rows: 3,
      cols: 7,
    })
    expect(focusedPos()).toEqual({ row: 2, col: 6 })
  })

  it("ArrowDown wraps from last row", () => {
    const root = makeGrid(3, 7)
    const start = root.querySelector(
      '[data-cell-pos="2-3"] button'
    ) as HTMLButtonElement
    start.focus()
    handleGridArrowKey(makeKeyEvent("ArrowDown", start, root) as never, {
      rows: 3,
      cols: 7,
    })
    expect(focusedPos()).toEqual({ row: 0, col: 3 })
  })

  it("Home jumps to column 0", () => {
    const root = makeGrid(3, 7)
    const start = root.querySelector(
      '[data-cell-pos="1-5"] button'
    ) as HTMLButtonElement
    start.focus()
    handleGridArrowKey(makeKeyEvent("Home", start, root) as never, {
      rows: 3,
      cols: 7,
    })
    expect(focusedPos()).toEqual({ row: 1, col: 0 })
  })

  it("End jumps to last column", () => {
    const root = makeGrid(3, 7)
    const start = root.querySelector(
      '[data-cell-pos="2-1"] button'
    ) as HTMLButtonElement
    start.focus()
    handleGridArrowKey(makeKeyEvent("End", start, root) as never, {
      rows: 3,
      cols: 7,
    })
    expect(focusedPos()).toEqual({ row: 2, col: 6 })
  })

  it("ignores arrow keys with modifiers", () => {
    const root = makeGrid(3, 7)
    const start = root.querySelector(
      '[data-cell-pos="0-0"] button'
    ) as HTMLButtonElement
    start.focus()
    const e = makeKeyEvent("ArrowRight", start, root)
    e.metaKey = true
    const handled = handleGridArrowKey(e as never, { rows: 3, cols: 7 })
    expect(handled).toBe(false)
    expect(focusedPos()).toEqual({ row: 0, col: 0 })
  })

  it("non-arrow keys are not handled", () => {
    const root = makeGrid(3, 7)
    const start = root.querySelector(
      '[data-cell-pos="0-0"] button'
    ) as HTMLButtonElement
    start.focus()
    const e = makeKeyEvent("a", start, root)
    expect(handleGridArrowKey(e as never, { rows: 3, cols: 7 })).toBe(false)
  })
})

describe("isCheatsheetShortcut", () => {
  it("matches plain ?", () => {
    const e = new KeyboardEvent("keydown", { key: "?" })
    expect(isCheatsheetShortcut(e)).toBe(true)
  })

  it("ignores ? with modifier (e.g. browser find)", () => {
    const e = new KeyboardEvent("keydown", { key: "?", metaKey: true })
    expect(isCheatsheetShortcut(e)).toBe(false)
  })

  it("ignores ? typed into an input", () => {
    const input = document.createElement("input")
    document.body.appendChild(input)
    input.focus()
    const e = new KeyboardEvent("keydown", { key: "?" })
    Object.defineProperty(e, "target", { value: input })
    expect(isCheatsheetShortcut(e)).toBe(false)
  })

  it("ignores ? in a contenteditable", () => {
    const div = document.createElement("div")
    div.setAttribute("contenteditable", "true")
    document.body.appendChild(div)
    const e = new KeyboardEvent("keydown", { key: "?" })
    Object.defineProperty(e, "target", { value: div })
    expect(isCheatsheetShortcut(e)).toBe(false)
  })
})

describe("isAcceptShortcut", () => {
  it("F2-AC7: matches ⌘+Enter and Ctrl+Enter", () => {
    for (const mod of [{ metaKey: true }, { ctrlKey: true }]) {
      expect(
        isAcceptShortcut(new KeyboardEvent("keydown", { key: "Enter", ...mod }))
      ).toBe(true)
    }
  })

  it("ignores plain Enter, other modifiers and editable fields", () => {
    expect(
      isAcceptShortcut(new KeyboardEvent("keydown", { key: "Enter" }))
    ).toBe(false)
    expect(
      isAcceptShortcut(
        new KeyboardEvent("keydown", {
          key: "Enter",
          ctrlKey: true,
          altKey: true,
        })
      )
    ).toBe(false)
    const area = document.createElement("textarea")
    const e = new KeyboardEvent("keydown", { key: "Enter", ctrlKey: true })
    Object.defineProperty(e, "target", { value: area })
    expect(isAcceptShortcut(e)).toBe(false)
  })

  it("ignores ⌘/Ctrl+Enter inside a dialog or sheet", () => {
    const dialog = document.createElement("div")
    dialog.setAttribute("role", "dialog")
    const button = document.createElement("button")
    dialog.appendChild(button)
    const e = new KeyboardEvent("keydown", { key: "Enter", metaKey: true })
    Object.defineProperty(e, "target", { value: button })
    expect(isAcceptShortcut(e)).toBe(false)
  })
})

describe("cellShortcut", () => {
  it("F2-AC7: maps N, R and S in either case", () => {
    const at = (key: string) =>
      cellShortcut({ key, metaKey: false, ctrlKey: false, altKey: false })
    expect(at("n")).toBe("no_plate")
    expect(at("R")).toBe("replace")
    expect(at("s")).toBe("side")
    expect(at("x")).toBeNull()
  })

  it("leaves modified keys to the browser", () => {
    expect(
      cellShortcut({ key: "r", metaKey: true, ctrlKey: false, altKey: false })
    ).toBeNull()
  })
})
