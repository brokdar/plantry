import { act, fireEvent, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import "@/lib/i18n"

import { BatchBadge } from "./BatchBadge"
import {
  HIGHLIGHT_MS,
  useBatchCell,
  useBatchHighlight,
} from "./batch-highlight"
import { BatchHighlightProvider } from "./BatchHighlightProvider"

function Cell({
  label,
  sessions,
  cooks = [],
}: {
  label: string
  sessions: number[]
  cooks?: number[]
}) {
  const props = useBatchCell(sessions, cooks)
  return (
    <div data-testid={label} {...props}>
      {label}
    </div>
  )
}

function Board() {
  return (
    <BatchHighlightProvider>
      <BatchBadge sessionId={7} colour={1} variant="cook" portions={6} />
      <BatchBadge sessionId={8} colour={2} variant="cook" portions={2} />
      <Cell label="sun-dinner" sessions={[7]} cooks={[7]} />
      <Cell label="mon-lunch" sessions={[7]} />
      <Cell label="mon-dinner" sessions={[7, 8]} cooks={[8]} />
      <Cell label="tue-lunch" sessions={[8]} />
      <Cell label="wed-lunch" sessions={[]} />
    </BatchHighlightProvider>
  )
}

const highlighted = () =>
  ["sun-dinner", "mon-lunch", "mon-dinner", "tue-lunch", "wed-lunch"].filter(
    (id) => screen.getByTestId(id).hasAttribute("data-highlighted")
  )

describe("BatchHighlightProvider (F2-AC3)", () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it("hovering a badge highlights every registered cell of its session for 2 s", () => {
    render(<Board />)
    expect(highlighted()).toEqual([])

    fireEvent.pointerEnter(screen.getByText("Cook × 6"), {
      pointerType: "mouse",
    })
    expect(highlighted()).toEqual(["sun-dinner", "mon-lunch", "mon-dinner"])
    // The value is the batch colour, so the ring takes it.
    expect(screen.getByTestId("mon-lunch")).toHaveAttribute(
      "data-highlighted",
      "1"
    )

    act(() => vi.advanceTimersByTime(HIGHLIGHT_MS - 1))
    expect(highlighted()).toEqual(["sun-dinner", "mon-lunch", "mon-dinner"])

    act(() => vi.advanceTimersByTime(1))
    expect(highlighted()).toEqual([])
  })

  it("a tap (touch pointer) highlights the same way", () => {
    render(<Board />)
    fireEvent.pointerEnter(screen.getByText("Cook × 2"), {
      pointerType: "touch",
    })
    expect(highlighted()).toEqual(["mon-dinner", "tue-lunch"])
    expect(screen.getByTestId("mon-dinner")).toHaveAttribute(
      "data-highlighted",
      "2"
    )
  })

  it("a second badge replaces the highlight and restarts the 2 s", () => {
    render(<Board />)
    fireEvent.pointerEnter(screen.getByText("Cook × 6"))
    act(() => vi.advanceTimersByTime(1500))
    fireEvent.pointerEnter(screen.getByText("Cook × 2"))
    expect(highlighted()).toEqual(["mon-dinner", "tue-lunch"])

    act(() => vi.advanceTimersByTime(1500))
    expect(highlighted()).toEqual(["mon-dinner", "tue-lunch"])
    act(() => vi.advanceTimersByTime(500))
    expect(highlighted()).toEqual([])
  })

  it("reveal scrolls the session's cook cell into view and highlights the batch", () => {
    const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView")
    function RevealButton() {
      const { reveal } = useBatchHighlight()
      return (
        <button type="button" onClick={() => reveal(8, 2)}>
          reveal
        </button>
      )
    }
    render(
      <BatchHighlightProvider>
        <RevealButton />
        <Cell label="mon-dinner" sessions={[7, 8]} cooks={[8]} />
        <Cell label="tue-lunch" sessions={[8]} />
      </BatchHighlightProvider>
    )
    fireEvent.click(screen.getByText("reveal"))
    expect(scroll).toHaveBeenCalledTimes(1)
    expect(scroll.mock.contexts[0]).toBe(screen.getByTestId("mon-dinner"))
    expect(screen.getByTestId("tue-lunch")).toHaveAttribute(
      "data-highlighted",
      "2"
    )
    scroll.mockRestore()
  })

  it("badges and cells work without a provider (no highlight)", () => {
    render(
      <>
        <BatchBadge sessionId={7} colour={1} variant="cook" portions={6} />
        <Cell label="sun-dinner" sessions={[7]} cooks={[7]} />
      </>
    )
    fireEvent.pointerEnter(screen.getByText("Cook × 6"))
    expect(screen.getByTestId("sun-dinner")).not.toHaveAttribute(
      "data-highlighted"
    )
  })
})
